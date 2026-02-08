// Package rpc defines the shared wire types for communication between
// the host (zeitwerk daemon), the guest (initagent), and the CLI (zeitworkctl).
package rpc

// ConfigResponse is the JSON response for GET /config (host -> guest).
type ConfigResponse struct {
	Env      []string `json:"env"`
	IPAddr   string   `json:"ip_addr"`
	IPGW     string   `json:"ip_gw"`
	Hostname string   `json:"hostname"`
}

// ExecRequest is the initial WebSocket text message for an exec session.
type ExecRequest struct {
	Command []string `json:"command"`
	TTY     bool     `json:"tty"`
}

// ExecControl is a WebSocket text message for control signals (resize, exit).
type ExecControl struct {
	Resize *ExecResize `json:"resize,omitempty"`
	Exit   *int        `json:"exit,omitempty"`
}

// ExecResize describes a terminal resize event.
type ExecResize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}
