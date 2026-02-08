package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"

	"github.com/mdlayher/vsock"
	"github.com/zeitwork/zeitwork/internal/rpc"
)

// vsockHTTPClient returns an http.Client that dials the host via AF_VSOCK.
func vsockHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return vsock.Dial(hostCID, agentPort, nil)
			},
		},
	}
}

// fetchConfig calls GET /config on the host and returns the parsed response.
func fetchConfig() rpc.ConfigResponse {
	client := vsockHTTPClient()

	resp, err := client.Get("http://host/config")
	checkErr(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		checkErr(fmt.Errorf("GET /config returned %d: %s", resp.StatusCode, body))
	}

	var config rpc.ConfigResponse
	checkErr(json.NewDecoder(resp.Body).Decode(&config))
	return config
}

// startLogStream opens a long-lived POST /logs to the host and returns a writer.
// Each line written is sent as a raw text line to the host.
// Close the writer to end the stream.
func startLogStream() io.WriteCloser {
	pr, pw := io.Pipe()

	go func() {
		client := vsockHTTPClient()
		resp, err := client.Post("http://host/logs", "text/plain", pr)
		if err != nil {
			slog.Error("log stream POST failed", "err", err)
			return
		}
		defer resp.Body.Close()
		slog.Debug("log stream ended", "status", resp.StatusCode)
	}()

	return pw
}
