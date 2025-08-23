package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageBuilder handles building container images from GitHub repositories
type ImageBuilder struct {
	server *Server
}

// NewImageBuilder creates a new image builder
func NewImageBuilder(server *Server) *ImageBuilder {
	return &ImageBuilder{
		server: server,
	}
}

// StartBuild starts building an image from a GitHub repository
func (ib *ImageBuilder) StartBuild(githubRepo string, tag string, name string) *Image {
	// Create image record
	image := &Image{
		ID:         generateID("img"),
		Name:       name,
		GitHubRepo: githubRepo,
		Tag:        tag,
		Status:     "building",
		CreatedAt:  time.Now(),
	}

	// Add to server state
	ib.server.mu.Lock()
	ib.server.images[image.ID] = image
	ib.server.mu.Unlock()

	// Start build in background
	go ib.buildImageAsync(image)

	return image
}

// buildImageAsync builds the image asynchronously
func (ib *ImageBuilder) buildImageAsync(image *Image) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in buildImageAsync for image %s: %v", image.ID, r)
			image.Status = "failed"
			image.BuildLog += fmt.Sprintf("\nBuild failed with panic: %v", r)
		}
	}()

	log.Printf("Starting build for image %s from GitHub repo %s", image.ID, image.GitHubRepo)

	// Select a node for building (preferably one with most resources)
	node := ib.selectBuildNode()
	if node == nil {
		image.Status = "failed"
		image.BuildLog = "No available nodes for building"
		log.Printf("No available nodes for building image %s", image.ID)
		return
	}

	// Build the image on the selected node
	if err := ib.buildOnNode(image, node); err != nil {
		image.Status = "failed"
		image.BuildLog += fmt.Sprintf("\nBuild failed: %v", err)
		log.Printf("Failed to build image %s: %v", image.ID, err)
		return
	}

	image.Status = "ready"
	log.Printf("Successfully built image %s", image.ID)
}

// selectBuildNode selects the best node for building an image
func (ib *ImageBuilder) selectBuildNode() *Node {
	ib.server.mu.RLock()
	defer ib.server.mu.RUnlock()

	var bestNode *Node
	maxAvailableMem := 0

	for _, node := range ib.server.nodes {
		if node.Status == "online" && node.Resources.MemoryMiBAvailable > maxAvailableMem {
			bestNode = node
			maxAvailableMem = node.Resources.MemoryMiBAvailable
		}
	}

	return bestNode
}

// buildOnNode builds the image on a specific node
func (ib *ImageBuilder) buildOnNode(image *Image, node *Node) error {
	buildDir := fmt.Sprintf("/tmp/build-%s", image.ID)
	imageFile := fmt.Sprintf("/var/lib/firecracker/images/%s.tar", image.ID)

	// Get the build script path
	scriptLocalPath := ib.getBuildScriptPath()
	scriptRemotePath := "/tmp/build_image.sh"

	// Read the build script
	scriptContent, err := os.ReadFile(scriptLocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("build script not found at %s. Please ensure you're running from the project root or set ZEITWORK_ROOT environment variable", scriptLocalPath)
		}
		return fmt.Errorf("failed to read build script: %v", err)
	}

	// Upload the build script to the node
	log.Printf("Uploading build script to node %s", node.ID)
	if err := ib.server.nodeManager.UploadFile(node.ID, scriptContent, scriptRemotePath); err != nil {
		return fmt.Errorf("failed to upload build script: %v", err)
	}

	// Make the script executable
	makeExecCmd := fmt.Sprintf("chmod +x %s", scriptRemotePath)
	if _, err := ib.server.nodeManager.RunCommand(node.ID, makeExecCmd); err != nil {
		return fmt.Errorf("failed to make build script executable: %v", err)
	}

	// Initialize build log
	var buildLog bytes.Buffer
	buildLog.WriteString(fmt.Sprintf("=== Build started at %s ===\n", time.Now().Format(time.RFC3339)))
	buildLog.WriteString(fmt.Sprintf("Node: %s (%s)\n", node.Name, node.ID))
	buildLog.WriteString(fmt.Sprintf("Repository: %s\n", image.GitHubRepo))
	buildLog.WriteString(fmt.Sprintf("Tag/Branch: %s\n\n", image.Tag))

	// Determine the ref to use
	ref := "main"
	if image.Tag != "" {
		ref = image.Tag
	}

	// Run the build script with arguments
	buildCmd := fmt.Sprintf("%s %s %s %s %s %s",
		scriptRemotePath,
		image.ID,
		image.GitHubRepo,
		ref,
		buildDir,
		imageFile,
	)

	log.Printf("Executing build command on node %s: %s", node.ID, buildCmd)
	buildLog.WriteString("Executing build script...\n")
	buildLog.WriteString("----------------------------------------\n")

	// Run the build and capture output
	buildOutput, err := ib.server.nodeManager.RunCommand(node.ID, buildCmd)
	buildLog.WriteString(buildOutput)

	// Update the image build log
	image.BuildLog = buildLog.String()

	if err != nil {
		buildLog.WriteString("\n----------------------------------------\n")
		buildLog.WriteString(fmt.Sprintf("Build failed with error: %v\n", err))
		buildLog.WriteString(fmt.Sprintf("=== Build failed at %s ===\n", time.Now().Format(time.RFC3339)))
		image.BuildLog = buildLog.String()

		// Log the full output for debugging
		log.Printf("Build failed for image %s. Full output:\n%s", image.ID, buildLog.String())

		return fmt.Errorf("build script failed: %v", err)
	}

	// Get image size
	sizeCmd := fmt.Sprintf("stat -c%%s %s 2>/dev/null || echo 0", imageFile)
	sizeOutput, _ := ib.server.nodeManager.RunCommand(node.ID, sizeCmd)
	fmt.Sscanf(strings.TrimSpace(sizeOutput), "%d", &image.Size)

	buildLog.WriteString("\n----------------------------------------\n")
	buildLog.WriteString(fmt.Sprintf("Image size: %d bytes\n", image.Size))
	buildLog.WriteString(fmt.Sprintf("=== Build completed at %s ===\n", time.Now().Format(time.RFC3339)))
	image.BuildLog = buildLog.String()

	// Cleanup build directory
	cleanupCmd := fmt.Sprintf("rm -rf %s", buildDir)
	if _, err := ib.server.nodeManager.RunCommand(node.ID, cleanupCmd); err != nil {
		log.Printf("Warning: failed to cleanup build directory: %v", err)
	}

	return nil
}

// getBuildScriptPath returns the path to the build script
func (ib *ImageBuilder) getBuildScriptPath() string {
	// Try multiple strategies to find the script

	// Strategy 1: Check if we're running from the project root or cmd directory
	possiblePaths := []string{
		"internal/builder/scripts/build_image.sh",    // Running from project root
		"../internal/builder/scripts/build_image.sh", // Running from cmd/
		"./internal/builder/scripts/build_image.sh",  // Explicit current dir
	}

	// Try each possible path
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			log.Printf("Found build script at: %s", absPath)
			return absPath
		}
	}

	// Strategy 2: Try to find based on executable location (for compiled binary)
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)

		// Check if we're in a temp directory (go run)
		if strings.Contains(execDir, "go-build") {
			// For go run, use working directory
			wd, err := os.Getwd()
			if err == nil {
				scriptPath := filepath.Join(wd, "internal", "builder", "scripts", "build_image.sh")
				if _, err := os.Stat(scriptPath); err == nil {
					log.Printf("Found build script at: %s", scriptPath)
					return scriptPath
				}
			}
		} else {
			// For compiled binary, check relative to executable
			scriptPath := filepath.Join(execDir, "internal", "builder", "scripts", "build_image.sh")
			if _, err := os.Stat(scriptPath); err == nil {
				log.Printf("Found build script at: %s", scriptPath)
				return scriptPath
			}

			// If executable is in cmd/, go up one level
			if strings.HasSuffix(execDir, "cmd") {
				scriptPath = filepath.Join(filepath.Dir(execDir), "internal", "builder", "scripts", "build_image.sh")
				if _, err := os.Stat(scriptPath); err == nil {
					log.Printf("Found build script at: %s", scriptPath)
					return scriptPath
				}
			}
		}
	}

	// Strategy 3: Try ZEITWORK_ROOT environment variable
	if rootDir := os.Getenv("ZEITWORK_ROOT"); rootDir != "" {
		scriptPath := filepath.Join(rootDir, "internal", "builder", "scripts", "build_image.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			log.Printf("Found build script at: %s", scriptPath)
			return scriptPath
		}
	}

	// Fallback: return the most likely path and let the error be caught later
	log.Printf("Warning: Could not locate build script, using default path")
	return "internal/builder/scripts/build_image.sh"
}

// GetImage retrieves an image by ID
func (ib *ImageBuilder) GetImage(imageID string) (*Image, error) {
	ib.server.mu.RLock()
	defer ib.server.mu.RUnlock()

	image, exists := ib.server.images[imageID]
	if !exists {
		return nil, fmt.Errorf("image %s not found", imageID)
	}

	return image, nil
}

// DeleteImage deletes an image
func (ib *ImageBuilder) DeleteImage(imageID string) error {
	ib.server.mu.Lock()
	image, exists := ib.server.images[imageID]
	if !exists {
		ib.server.mu.Unlock()
		return fmt.Errorf("image %s not found", imageID)
	}
	delete(ib.server.images, imageID)
	ib.server.mu.Unlock()

	// Delete the image file from all nodes
	imageFile := fmt.Sprintf("/var/lib/firecracker/images/%s.tar", imageID)

	ib.server.mu.RLock()
	nodes := make([]*Node, 0, len(ib.server.nodes))
	for _, node := range ib.server.nodes {
		nodes = append(nodes, node)
	}
	ib.server.mu.RUnlock()

	for _, node := range nodes {
		if node.Status == "online" {
			deleteCmd := fmt.Sprintf("rm -f %s", imageFile)
			if _, err := ib.server.nodeManager.RunCommand(node.ID, deleteCmd); err != nil {
				log.Printf("Warning: failed to delete image file from node %s: %v", node.ID, err)
			}
		}
	}

	log.Printf("Deleted image %s", image.ID)
	return nil
}

// ListImages returns all images
func (ib *ImageBuilder) ListImages() []*Image {
	ib.server.mu.RLock()
	defer ib.server.mu.RUnlock()

	images := make([]*Image, 0, len(ib.server.images))
	for _, image := range ib.server.images {
		images = append(images, image)
	}

	return images
}
