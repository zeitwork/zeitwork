package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/coder/websocket"
	"github.com/creack/pty/v2"
	"github.com/mdlayher/vsock"
	"github.com/zeitwork/zeitwork/internal/rpc"
)

// startGuestServer listens on VSOCK port 1025 for incoming connections from the host.
func startGuestServer() {
	lis, err := vsock.Listen(execPort, nil)
	if err != nil {
		slog.Error("failed to listen on guest VSOCK port", "port", execPort, "err", err)
		return
	}
	defer lis.Close()

	slog.Info("guest server listening", "port", execPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/exec", handleExec)

	srv := &http.Server{Handler: mux}
	if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("guest server error", "err", err)
	}
}

// handleExec upgrades an HTTP request to a WebSocket and runs an exec session.
func handleExec(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("websocket accept failed", "err", err)
		return
	}
	defer conn.CloseNow()

	ctx := r.Context()

	// Read the first message: ExecRequest (JSON, text)
	typ, data, err := conn.Read(ctx)
	if err != nil {
		slog.Error("failed to read exec request", "err", err)
		return
	}
	if typ != websocket.MessageText {
		slog.Error("expected text message for exec request, got binary")
		return
	}

	var req rpc.ExecRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.Error("failed to parse exec request", "err", err)
		return
	}

	if len(req.Command) == 0 {
		sendExecExit(conn, ctx, 1)
		return
	}

	slog.Info("exec session starting", "command", req.Command, "tty", req.TTY)

	// Wrap command with nsenter to join the customer app's PID+mount namespace,
	// dropping to the same UID/GID as the customer process.
	nsenterArgs := []string{
		"nsenter",
		"--target", strconv.Itoa(customerPID),
		"--pid", "--mount",
		"--setuid", strconv.FormatUint(uint64(customerUID), 10),
		"--setgid", strconv.FormatUint(uint64(customerGID), 10),
		"--",
	}
	cmd := &exec.Cmd{
		Path: "/.zeitwork/busybox",
		Args: append(nsenterArgs, req.Command...),
		Env:  os.Environ(),
	}

	if req.TTY {
		handleExecTTY(conn, ctx, cmd)
	} else {
		handleExecPipes(conn, ctx, cmd)
	}
}

// handleExecTTY runs the command with a PTY, merging stdout/stderr.
func handleExecTTY(conn *websocket.Conn, ctx context.Context, cmd *exec.Cmd) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		slog.Error("failed to start command with pty", "err", err)
		sendExecExit(conn, ctx, 1)
		return
	}
	defer ptmx.Close()

	// Relay PTY output to WebSocket (binary messages)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Relay WebSocket messages to PTY (binary=stdin, text=control)
	go func() {
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			switch typ {
			case websocket.MessageBinary:
				if _, err := ptmx.Write(data); err != nil {
					return
				}
			case websocket.MessageText:
				var ctrl rpc.ExecControl
				if err := json.Unmarshal(data, &ctrl); err != nil {
					continue
				}
				if ctrl.Resize != nil {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Rows: ctrl.Resize.Rows,
						Cols: ctrl.Resize.Cols,
					})
				}
			}
		}
	}()

	wg.Wait()

	exitCode := 0
	if waitErr := cmd.Wait(); waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	slog.Info("exec session ended", "exit_code", exitCode)
	sendExecExit(conn, ctx, exitCode)
}

// handleExecPipes runs the command with separate stdin/stdout/stderr pipes.
func handleExecPipes(conn *websocket.Conn, ctx context.Context, cmd *exec.Cmd) {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		slog.Error("failed to create stdin pipe", "err", err)
		sendExecExit(conn, ctx, 1)
		return
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("failed to create stdout pipe", "err", err)
		sendExecExit(conn, ctx, 1)
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		slog.Error("failed to create stderr pipe", "err", err)
		sendExecExit(conn, ctx, 1)
		return
	}

	if err := cmd.Start(); err != nil {
		slog.Error("failed to start exec command", "err", err)
		sendExecExit(conn, ctx, 1)
		return
	}

	// Relay WebSocket binary messages to stdin
	go func() {
		defer stdinPipe.Close()
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				if _, err := stdinPipe.Write(data); err != nil {
					return
				}
			}
		}
	}()

	// Relay stdout+stderr to WebSocket (merged as binary)
	var wg sync.WaitGroup
	wg.Add(2)

	relayPipe := func(pipe io.ReadCloser) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := pipe.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go relayPipe(stdoutPipe)
	go relayPipe(stderrPipe)

	wg.Wait()

	exitCode := 0
	if waitErr := cmd.Wait(); waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	slog.Info("exec session ended", "exit_code", exitCode)
	sendExecExit(conn, ctx, exitCode)
}

func sendExecExit(conn *websocket.Conn, ctx context.Context, exitCode int) {
	ctrl := rpc.ExecControl{Exit: &exitCode}
	data, _ := json.Marshal(ctrl)
	conn.Write(ctx, websocket.MessageText, data)
	conn.Close(websocket.StatusNormalClosure, "")
}
