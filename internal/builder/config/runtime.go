package config

// DockerRuntimeConfig holds configuration for Docker build runtime
type DockerRuntimeConfig struct {
	WorkDir          string // Directory where builds are performed
	Registry         string // Container registry to push images to
	InsecureRegistry bool   // Whether to allow insecure (HTTP) registry connections
}

// FirecrackerRuntimeConfig holds configuration for Firecracker build runtime
type FirecrackerRuntimeConfig struct {
	WorkDir  string // Directory where builds are performed
	Registry string // Container registry to push images to
	// TODO: Add firecracker-specific fields like:
	// NodeAgentEndpoint string
	// VMImage           string
	// VMResources       *VMResources
}
