package zeitwork

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// dialedConn wraps a net.Conn with a bufio.Reader so that any bytes
// buffered during the CONNECT handshake are not lost.
type dialedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *dialedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// DialGuest connects to a guest VM via the Cloud Hypervisor VSOCK bridge.
// It connects to the base VSOCK UDS, sends "CONNECT <port>\n", and reads
// the "OK <cid>\n" acknowledgment from Cloud Hypervisor.
// After the handshake, the returned net.Conn is a raw bidirectional stream
// to the guest process listening on that VSOCK port.
func DialGuest(vmID uuid.UUID, port int) (net.Conn, error) {
	basePath := VSocketPath(vmID)
	conn, err := net.Dial("unix", basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to VSOCK base socket %s: %w", basePath, err)
	}

	// Cloud Hypervisor expects "CONNECT <port>\n" to route to the guest listener.
	_, err = fmt.Fprintf(conn, "CONNECT %d\n", port)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT handshake: %w", err)
	}

	// Cloud Hypervisor responds with "OK <cid>\n" on success.
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read CONNECT response: %w", err)
	}

	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "OK ") {
		conn.Close()
		return nil, fmt.Errorf("CONNECT failed: %s", line)
	}

	return &dialedConn{Conn: conn, r: br}, nil
}
