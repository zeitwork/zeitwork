package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/coder/websocket"
	"github.com/zeitwork/zeitwork/internal/rpc"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"github.com/zeitwork/zeitwork/internal/zeitwork"
	"golang.org/x/term"
)

const (
	// execPort is the VSOCK port the guest listens on for exec.
	execPort = 1025
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "exec":
		os.Exit(cmdExec(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: zeitworkctl <command> [args]

Commands:
  exec <vm-id> -- <command...>    Execute a command inside a VM
`)
}

func cmdExec(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: zeitworkctl exec <vm-id> [-- <command...>]")
		return 1
	}

	vmID, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid vm-id: %v\n", err)
		return 1
	}

	// Find command after "--"
	var command []string
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			command = args[i+1:]
			break
		}
	}
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}

	stdinFd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(stdinFd)

	// Connect directly to the VM via VSOCK
	rawConn, err := zeitwork.DialGuest(vmID, execPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to VM: %v\n", err)
		return 1
	}
	defer rawConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// WebSocket upgrade over the raw VSOCK connection.
	conn, _, err := websocket.Dial(ctx, "ws://guest/exec", &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return rawConn, nil
				},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "websocket handshake failed: %v\r\n", err)
		return 1
	}
	defer conn.CloseNow()

	// Disable read limit (default is 32KB which is too small for large outputs)
	conn.SetReadLimit(-1)

	// Put terminal into raw mode for interactive use
	if isTTY {
		oldState, err := term.MakeRaw(stdinFd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to set raw mode: %v\n", err)
			return 1
		}
		defer term.Restore(stdinFd, oldState)
	}

	// Send ExecRequest as first text message
	reqData, _ := json.Marshal(rpc.ExecRequest{
		Command: command,
		TTY:     isTTY,
	})
	if err := conn.Write(ctx, websocket.MessageText, reqData); err != nil {
		fmt.Fprintf(os.Stderr, "failed to send exec request: %v\r\n", err)
		return 1
	}

	// Send initial terminal size
	if isTTY {
		sendTermSize(conn, ctx, stdinFd)
	}

	// Handle SIGWINCH (terminal resize)
	if isTTY {
		winchCh := make(chan os.Signal, 1)
		signal.Notify(winchCh, syscall.SIGWINCH)
		go func() {
			for range winchCh {
				sendTermSize(conn, ctx, stdinFd)
			}
		}()
	}

	// Handle SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Relay stdin to WebSocket in background (binary messages)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
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

	// Receive output from WebSocket
	exitCode := 0
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, io.EOF) && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				fmt.Fprintf(os.Stderr, "stream error: %v\r\n", err)
			}
			break
		}

		switch typ {
		case websocket.MessageBinary:
			// stdout/stderr data
			os.Stdout.Write(data)
		case websocket.MessageText:
			// control message (exit code)
			var ctrl rpc.ExecControl
			if err := json.Unmarshal(data, &ctrl); err != nil {
				continue
			}
			if ctrl.Exit != nil {
				exitCode = *ctrl.Exit
			}
		}
	}

	return exitCode
}

func sendTermSize(conn *websocket.Conn, ctx context.Context, fd int) {
	w, h, err := term.GetSize(fd)
	if err != nil {
		return
	}
	ctrl := rpc.ExecControl{
		Resize: &rpc.ExecResize{
			Rows: uint16(h),
			Cols: uint16(w),
		},
	}
	data, _ := json.Marshal(ctrl)
	_ = conn.Write(ctx, websocket.MessageText, data)
}
