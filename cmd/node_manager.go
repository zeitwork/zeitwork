package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// NodeManager handles node operations and SSH connections
type NodeManager struct {
	server *Server
	mu     sync.RWMutex

	// SSH clients cache
	sshClients map[string]*ssh.Client
}

// NewNodeManager creates a new node manager
func NewNodeManager(server *Server) *NodeManager {
	return &NodeManager{
		server:     server,
		sshClients: make(map[string]*ssh.Client),
	}
}

// AddNode adds a new node to the cluster
func (nm *NodeManager) AddNode(node *Node) (*Node, error) {
	// Generate ID if not provided
	if node.ID == "" {
		node.ID = generateID("node")
	}

	// Set default name if not provided
	if node.Name == "" {
		node.Name = fmt.Sprintf("node-%s", node.Host)
	}

	// Check SSH key
	if node.SSHKeyPath == "" {
		keyPath, _ := getDefaultKeyPath()
		if checkSSHKey(keyPath) {
			node.SSHKeyPath = keyPath
		} else {
			// Generate new SSH key
			log.Printf("Generating SSH key for node %s", node.ID)
			node.SSHKeyPath = generateSSHKey()
		}
	}

	// Test connection
	client, err := nm.createSSHClient(node)
	if err != nil {
		node.Status = "offline"
		log.Printf("Failed to connect to node %s: %v", node.ID, err)
	} else {
		node.Status = "online"
		node.LastPing = time.Now()

		// Get node resources
		if err := nm.updateNodeResources(node, client); err != nil {
			log.Printf("Failed to get resources for node %s: %v", node.ID, err)
		}

		// Cache the client
		nm.mu.Lock()
		nm.sshClients[node.ID] = client
		nm.mu.Unlock()
	}

	// Add to server
	nm.server.mu.Lock()
	nm.server.nodes[node.ID] = node
	nm.server.mu.Unlock()

	return node, nil
}

// GetSSHClient gets or creates an SSH client for a node
func (nm *NodeManager) GetSSHClient(nodeID string) (*ssh.Client, error) {
	nm.mu.RLock()
	client, exists := nm.sshClients[nodeID]
	nm.mu.RUnlock()

	if exists && client != nil {
		// Test if connection is still alive
		session, err := client.NewSession()
		if err == nil {
			session.Close()
			return client, nil
		}
		// Connection is dead, remove it
		nm.mu.Lock()
		delete(nm.sshClients, nodeID)
		nm.mu.Unlock()
	}

	// Get node
	nm.server.mu.RLock()
	node, exists := nm.server.nodes[nodeID]
	nm.server.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	// Create new connection
	client, err := nm.createSSHClient(node)
	if err != nil {
		return nil, err
	}

	// Cache it
	nm.mu.Lock()
	nm.sshClients[nodeID] = client
	nm.mu.Unlock()

	return client, nil
}

// createSSHClient creates an SSH client for a node
func (nm *NodeManager) createSSHClient(node *Node) (*ssh.Client, error) {
	key, err := os.ReadFile(node.SSHKeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", node.Host, node.Port)
	return ssh.Dial("tcp", address, sshConfig)
}

// RunCommand runs a command on a node
func (nm *NodeManager) RunCommand(nodeID string, command string) (string, error) {
	client, err := nm.GetSSHClient(nodeID)
	if err != nil {
		return "", err
	}

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}

// UploadFile uploads content to a file on the node
func (nm *NodeManager) UploadFile(nodeID string, content []byte, remotePath string) error {
	client, err := nm.GetSSHClient(nodeID)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = bytes.NewReader(content)
	command := fmt.Sprintf("cat > %s", remotePath)

	return session.Run(command)
}

// updateNodeResources updates the resource information for a node
func (nm *NodeManager) updateNodeResources(node *Node, client *ssh.Client) error {
	// Get CPU count
	cpuCmd := "nproc"
	output, err := nm.runCommandWithClient(client, cpuCmd)
	if err == nil {
		var cpuCount int
		fmt.Sscanf(strings.TrimSpace(output), "%d", &cpuCount)
		node.Resources.VCPUTotal = cpuCount
		node.Resources.VCPUAvailable = cpuCount // TODO: Calculate based on running instances
	}

	// Get memory
	memCmd := "free -m | grep '^Mem:' | awk '{print $2}'"
	output, err = nm.runCommandWithClient(client, memCmd)
	if err == nil {
		var memMiB int
		fmt.Sscanf(strings.TrimSpace(output), "%d", &memMiB)
		node.Resources.MemoryMiBTotal = memMiB
		node.Resources.MemoryMiBAvailable = memMiB // TODO: Calculate based on running instances
	}

	return nil
}

// runCommandWithClient runs a command using an existing SSH client
func (nm *NodeManager) runCommandWithClient(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}

// StartHealthChecker starts a background goroutine to check node health
func (nm *NodeManager) StartHealthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		nm.checkAllNodes()
	}
}

// checkAllNodes checks the health of all nodes
func (nm *NodeManager) checkAllNodes() {
	nm.server.mu.RLock()
	nodes := make([]*Node, 0, len(nm.server.nodes))
	for _, node := range nm.server.nodes {
		nodes = append(nodes, node)
	}
	nm.server.mu.RUnlock()

	for _, node := range nodes {
		go nm.checkNode(node)
	}
}

// checkNode checks the health of a single node
func (nm *NodeManager) checkNode(node *Node) {
	client, err := nm.GetSSHClient(node.ID)
	if err != nil {
		node.Status = "offline"
		return
	}

	// Simple health check
	output, err := nm.runCommandWithClient(client, "echo OK")
	if err != nil || !strings.Contains(output, "OK") {
		node.Status = "offline"
	} else {
		node.Status = "online"
		node.LastPing = time.Now()

		// Update resources
		nm.updateNodeResources(node, client)
	}
}

// Helper functions (from original code)

func getDefaultKeyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ssh", "firecracker_manager_rsa"), nil
}

func checkSSHKey(keyPath string) bool {
	if _, err := os.Stat(keyPath); err != nil {
		return false
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		return false
	}
	return true
}

func generateSSHKey() string {
	keyPath, err := getDefaultKeyPath()
	if err != nil {
		log.Fatalf("Failed to get default key path: %v", err)
	}

	sshDir := filepath.Dir(keyPath)

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		log.Fatalf("Failed to create .ssh directory: %v", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %v", err)
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	privateKeyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to create private key file: %v", err)
	}
	defer privateKeyFile.Close()

	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		log.Fatalf("Failed to write private key: %v", err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		log.Fatalf("Failed to generate SSH public key: %v", err)
	}

	hostname, _ := os.Hostname()
	publicKeyString := fmt.Sprintf("%s firecracker-manager@%s\n",
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))),
		hostname)

	publicKeyPath := keyPath + ".pub"
	if err := os.WriteFile(publicKeyPath, []byte(publicKeyString), 0644); err != nil {
		log.Fatalf("Failed to write public key: %v", err)
	}

	return keyPath
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
