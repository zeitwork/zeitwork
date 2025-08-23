package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Data structures
type Node struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	SSHKeyPath string    `json:"ssh_key_path,omitempty"`
	Status     string    `json:"status"` // "online", "offline", "unknown"
	LastPing   time.Time `json:"last_ping"`
	Resources  Resources `json:"resources"`
}

type Resources struct {
	VCPUTotal          int `json:"vcpu_total"`
	VCPUAvailable      int `json:"vcpu_available"`
	MemoryMiBTotal     int `json:"memory_mib_total"`
	MemoryMiBAvailable int `json:"memory_mib_available"`
}

type Instance struct {
	ID          string    `json:"id"`
	NodeID      string    `json:"node_id"`
	ImageID     string    `json:"image_id"`
	Status      string    `json:"status"` // "running", "stopped", "creating", "error"
	VCPUCount   int       `json:"vcpu_count"`
	MemoryMiB   int       `json:"memory_mib"`
	DefaultPort int       `json:"default_port,omitempty"` // Default port to expose
	CreatedAt   time.Time `json:"created_at"`
	IPAddress   string    `json:"ip_address,omitempty"`
}

type Image struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	GitHubRepo string    `json:"github_repo,omitempty"`
	Tag        string    `json:"tag,omitempty"`
	Status     string    `json:"status"` // "building", "ready", "failed"
	Size       int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
	BuildLog   string    `json:"build_log,omitempty"`
}

type BuildImageRequest struct {
	GitHubRepo string `json:"github_repo"` // e.g., "owner/repo"
	Tag        string `json:"tag,omitempty"`
	Name       string `json:"name"`
}

type CreateInstanceRequest struct {
	NodeID      string `json:"node_id"`
	ImageID     string `json:"image_id"`
	VCPUCount   int    `json:"vcpu_count"`
	MemoryMiB   int    `json:"memory_mib"`
	DefaultPort int    `json:"default_port,omitempty"`
}

// ProxyInfo stores SSH tunnel information for an instance
type ProxyInfo struct {
	InstanceID string    `json:"instance_id"`
	LocalPort  int       `json:"local_port"`
	RemoteIP   string    `json:"remote_ip"`
	RemotePort int       `json:"remote_port"`
	Status     string    `json:"status"` // "active", "failed"
	CreatedAt  time.Time `json:"created_at"`
	AccessURL  string    `json:"access_url"`
}

// Server represents our API server
type Server struct {
	mu        sync.RWMutex
	nodes     map[string]*Node
	instances map[string]*Instance
	images    map[string]*Image
	proxies   map[string]*ProxyInfo // instance_id -> proxy info

	nodeManager     *NodeManager
	instanceManager *InstanceManager
	imageBuilder    *ImageBuilder
}

// NewServer creates a new API server
func NewServer() *Server {
	s := &Server{
		nodes:     make(map[string]*Node),
		instances: make(map[string]*Instance),
		images:    make(map[string]*Image),
		proxies:   make(map[string]*ProxyInfo),
	}

	s.nodeManager = NewNodeManager(s)
	s.instanceManager = NewInstanceManager(s)
	s.imageBuilder = NewImageBuilder(s)

	return s
}

// CleanupOrphanedResources cleans up resources from previous runs
func (s *Server) CleanupOrphanedResources() {
	log.Println("Cleaning up orphaned resources from previous runs...")

	// For each node, clean up orphaned Firecracker processes and TAP devices
	for _, node := range s.nodes {
		if node.Status != "online" {
			continue
		}

		cleanupCmd := `
			#!/bin/bash
			echo "Cleaning up orphaned resources on node..."
			
			# Find and kill orphaned Firecracker processes
			echo "Looking for orphaned Firecracker processes..."
			FC_PIDS=$(pgrep -x firecracker 2>/dev/null || true)
			if [ -n "$FC_PIDS" ]; then
				echo "Found Firecracker PIDs: $FC_PIDS"
				for PID in $FC_PIDS; do
					echo "Killing Firecracker PID $PID"
					kill -TERM $PID 2>/dev/null || true
				done
				sleep 2
				# Force kill if still running
				for PID in $FC_PIDS; do
					if ps -p $PID > /dev/null 2>&1; then
						echo "Force killing PID $PID"
						kill -KILL $PID 2>/dev/null || true
					fi
				done
			else
				echo "No orphaned Firecracker processes found"
			fi
			
			# Clean up orphaned TAP devices
			echo "Looking for orphaned TAP devices..."
			TAP_DEVICES=$(ip link show type tap 2>/dev/null | grep -oE '^[0-9]+: tap[^:]+' | cut -d' ' -f2 || true)
			if [ -n "$TAP_DEVICES" ]; then
				echo "Found TAP devices: $TAP_DEVICES"
				for TAP in $TAP_DEVICES; do
					echo "Removing TAP device $TAP"
					ip link set $TAP down 2>/dev/null || true
					ip link delete $TAP 2>/dev/null || true
				done
			else
				echo "No orphaned TAP devices found"
			fi
			
			# Clean up VM directories for instances we don't know about
			echo "Cleaning up orphaned VM directories..."
			if [ -d /var/lib/firecracker/vms ]; then
				for VM_DIR in /var/lib/firecracker/vms/instance-*; do
					if [ -d "$VM_DIR" ]; then
						echo "Removing orphaned VM directory: $VM_DIR"
						rm -rf "$VM_DIR"
					fi
				done
			fi
			
			# Clean up any leftover iptables rules
			echo "Cleaning up iptables rules..."
			# Remove all DNAT rules pointing to 10.0.0.2
			iptables -t nat -L PREROUTING -n --line-numbers | grep "10.0.0.2" | awk '{print $1}' | sort -rn | while read line; do
				iptables -t nat -D PREROUTING $line 2>/dev/null || true
			done
			# Remove all FORWARD rules for 10.0.0.2
			iptables -L FORWARD -n --line-numbers | grep "10.0.0.2" | awk '{print $1}' | sort -rn | while read line; do
				iptables -D FORWARD $line 2>/dev/null || true
			done
			
			echo "Cleanup complete"
		`

		output, err := s.nodeManager.RunCommand(node.ID, cleanupCmd)
		if err != nil {
			log.Printf("Failed to cleanup orphaned resources on node %s: %v", node.ID, err)
		} else {
			log.Printf("Cleanup output for node %s:\n%s", node.ID, output)
		}
	}
}

// HTTP Handlers

// handleGetNodes returns the list of all available nodes
func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	nodes := make([]*Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// handleGetInstances returns the list of all instances
func (s *Server) handleGetInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	instances := make([]*Instance, 0, len(s.instances))
	for _, instance := range s.instances {
		instances = append(instances, instance)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instances)
}

// handleCreateInstance creates a new VM instance with the specified image
func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the request
	if req.NodeID == "" || req.ImageID == "" {
		http.Error(w, "node_id and image_id are required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.VCPUCount == 0 {
		req.VCPUCount = 1
	}
	if req.MemoryMiB == 0 {
		req.MemoryMiB = 128
	}

	// Check if node exists
	s.mu.RLock()
	node, nodeExists := s.nodes[req.NodeID]
	image, imageExists := s.images[req.ImageID]
	s.mu.RUnlock()

	if !nodeExists {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	if !imageExists {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	if image.Status != "ready" {
		http.Error(w, fmt.Sprintf("Image is not ready (status: %s)", image.Status), http.StatusBadRequest)
		return
	}

	// Create the instance
	instance, err := s.instanceManager.CreateInstance(node, image, req.VCPUCount, req.MemoryMiB, req.DefaultPort)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create instance: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(instance)
}

// handleGetImages returns the list of all available images
func (s *Server) handleGetImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	images := make([]*Image, 0, len(s.images))
	for _, image := range s.images {
		images = append(images, image)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

// handleBuildImage builds a new image from a GitHub repository
func (s *Server) handleBuildImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BuildImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the request
	if req.GitHubRepo == "" {
		http.Error(w, "github_repo is required", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		req.Name = req.GitHubRepo
	}

	// Start building the image asynchronously
	image := s.imageBuilder.StartBuild(req.GitHubRepo, req.Tag, req.Name)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(image)
}

// handleAddNode adds a new node to the cluster
func (s *Server) handleAddNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var node Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the node
	if node.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}

	if node.Port == 0 {
		node.Port = 22
	}

	// Add the node
	addedNode, err := s.nodeManager.AddNode(&node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to add node: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(addedNode)
}

// handleSetupInstanceProxy sets up an SSH tunnel for an instance
func (s *Server) handleSetupInstanceProxy(w http.ResponseWriter, r *http.Request, instanceID string) {
	// Check if instance exists
	s.mu.RLock()
	instance, exists := s.instances[instanceID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	if instance.Status != "running" {
		http.Error(w, fmt.Sprintf("Instance is not running (status: %s)", instance.Status), http.StatusBadRequest)
		return
	}

	// Check if proxy already exists
	s.mu.RLock()
	existingProxy, proxyExists := s.proxies[instanceID]
	s.mu.RUnlock()

	if proxyExists && existingProxy.Status == "active" {
		// Return existing proxy info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existingProxy)
		return
	}

	// Get node information
	s.mu.RLock()
	node, nodeExists := s.nodes[instance.NodeID]
	s.mu.RUnlock()

	if !nodeExists {
		http.Error(w, "Node not found", http.StatusInternalServerError)
		return
	}

	// Allocate a local port (starting from 9000)
	localPort := 9000 + (hashString(instanceID) % 1000)

	// Parse request body for optional port configuration
	var req struct {
		RemotePort int `json:"remote_port,omitempty"`
		LocalPort  int `json:"local_port,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.LocalPort > 0 {
		localPort = req.LocalPort
	}

	// Use instance's default port if set, otherwise use request's remote port, otherwise default to 8080
	remotePort := 8080 // Default application port
	if instance.DefaultPort > 0 {
		remotePort = instance.DefaultPort
	}
	if req.RemotePort > 0 {
		remotePort = req.RemotePort
	}

	// Kill any existing tunnel on this port
	exec.Command("pkill", "-f", fmt.Sprintf("ssh.*-L.*%d:", localPort)).Run()

	// Setup SSH tunnel
	sshCmd := exec.Command("ssh",
		"-N",
		"-L", fmt.Sprintf("%d:%s:%d", localPort, instance.IPAddress, remotePort),
		"-p", strconv.Itoa(node.Port),
		"-i", node.SSHKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		fmt.Sprintf("root@%s", node.Host),
	)

	log.Printf("Setting up SSH tunnel for instance %s: localhost:%d -> %s:%d",
		instanceID, localPort, instance.IPAddress, remotePort)

	if err := sshCmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to setup SSH tunnel: %v", err), http.StatusInternalServerError)
		return
	}

	// Give tunnel time to establish
	time.Sleep(2 * time.Second)

	// Create proxy info
	proxyInfo := &ProxyInfo{
		InstanceID: instanceID,
		LocalPort:  localPort,
		RemoteIP:   instance.IPAddress,
		RemotePort: remotePort,
		Status:     "active",
		CreatedAt:  time.Now(),
		AccessURL:  fmt.Sprintf("http://localhost:%d", localPort),
	}

	// Store proxy info
	s.mu.Lock()
	s.proxies[instanceID] = proxyInfo
	s.mu.Unlock()

	log.Printf("SSH proxy established for instance %s at %s", instanceID, proxyInfo.AccessURL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(proxyInfo)
}

// handleGetInstanceProxy returns proxy information for an instance
func (s *Server) handleGetInstanceProxy(w http.ResponseWriter, r *http.Request, instanceID string) {
	s.mu.RLock()
	proxy, exists := s.proxies[instanceID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "No proxy configured for this instance", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxy)
}

// handleDeleteInstanceProxy tears down the SSH tunnel for an instance
func (s *Server) handleDeleteInstanceProxy(w http.ResponseWriter, r *http.Request, instanceID string) {
	s.mu.RLock()
	proxy, exists := s.proxies[instanceID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "No proxy configured for this instance", http.StatusNotFound)
		return
	}

	// Kill SSH tunnel
	killCmd := exec.Command("pkill", "-f", fmt.Sprintf("ssh.*-L.*%d:", proxy.LocalPort))
	if err := killCmd.Run(); err != nil {
		log.Printf("Warning: failed to kill SSH tunnel: %v", err)
	}

	// Remove proxy info
	s.mu.Lock()
	delete(s.proxies, instanceID)
	s.mu.Unlock()

	log.Printf("SSH proxy removed for instance %s", instanceID)

	w.WriteHeader(http.StatusNoContent)
}

// handleGetInstanceLogs returns all logs for an instance
func (s *Server) handleGetInstanceLogs(w http.ResponseWriter, r *http.Request, instanceID string) {
	logs, err := s.instanceManager.GetInstanceLogs(instanceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return logs as JSON response
	response := struct {
		InstanceID string `json:"instance_id"`
		Logs       string `json:"logs"`
	}{
		InstanceID: instanceID,
		Logs:       logs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// hashString generates a hash integer from a string
func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Node endpoints
	mux.HandleFunc("/nodes", s.handleGetNodes)
	mux.HandleFunc("/nodes/add", s.handleAddNode)

	// Instance endpoints
	mux.HandleFunc("/instances", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetInstances(w, r)
		case http.MethodPost:
			s.handleCreateInstance(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Instance proxy and logs endpoints - matches /instances/{id}/proxy and /instances/{id}/logs
	mux.HandleFunc("/instances/", func(w http.ResponseWriter, r *http.Request) {
		// Extract instance ID and action from path
		path := strings.TrimPrefix(r.URL.Path, "/instances/")
		parts := strings.Split(path, "/")

		if len(parts) == 2 {
			instanceID := parts[0]
			action := parts[1]

			switch action {
			case "proxy":
				switch r.Method {
				case http.MethodPost:
					s.handleSetupInstanceProxy(w, r, instanceID)
				case http.MethodGet:
					s.handleGetInstanceProxy(w, r, instanceID)
				case http.MethodDelete:
					s.handleDeleteInstanceProxy(w, r, instanceID)
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			case "logs":
				if r.Method == http.MethodGet {
					s.handleGetInstanceLogs(w, r, instanceID)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			default:
				http.Error(w, "Not found", http.StatusNotFound)
			}
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	})

	// Image endpoints
	mux.HandleFunc("/images", s.handleGetImages)
	mux.HandleFunc("/images/build", s.handleBuildImage)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Cleanup endpoint - useful for manual cleanup
	mux.HandleFunc("/cleanup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Println("Manual cleanup requested")
		s.CleanupOrphanedResources()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleanup completed"})
	})

	return mux
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	mux := s.setupRoutes()

	// Clean up any orphaned resources from previous runs
	// Note: This only works if nodes have been added before Start is called
	// In production, you might want to call this after nodes are added
	if len(s.nodes) > 0 {
		s.CleanupOrphanedResources()
	}

	// Start background workers
	go s.nodeManager.StartHealthChecker()

	log.Printf("🚀 Firecracker Manager API Server starting on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func main() {
	server := NewServer()

	addr := ":8080"
	fmt.Println("🔥 Firecracker Manager API Server 🔥")
	fmt.Println("=====================================")
	fmt.Printf("Starting server on %s\n\n", addr)

	fmt.Println("Available endpoints:")
	fmt.Println("  GET    /nodes                    - List all nodes")
	fmt.Println("  POST   /nodes/add                - Add a new node")
	fmt.Println("  GET    /instances                - List all instances")
	fmt.Println("  POST   /instances                - Create a new instance")
	fmt.Println("  GET    /instances/{id}/logs      - Get all logs for an instance")
	fmt.Println("  POST   /instances/{id}/proxy     - Setup SSH proxy for instance")
	fmt.Println("  GET    /instances/{id}/proxy     - Get proxy info for instance")
	fmt.Println("  DELETE /instances/{id}/proxy     - Remove SSH proxy for instance")
	fmt.Println("  GET    /images                   - List all images")
	fmt.Println("  POST   /images/build             - Build image from GitHub repo")
	fmt.Println("  GET    /health                   - Health check")
	fmt.Println("  POST   /cleanup                  - Clean up orphaned resources")
	fmt.Println()

	if err := server.Start(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
