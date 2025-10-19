package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	HCloudToken            string `env:"HCLOUD_TOKEN"`
	DockerRegistryURL      string `env:"DOCKER_REGISTRY_URL"`
	DockerRegistryUsername string `env:"DOCKER_REGISTRY_USERNAME"`
	DockerRegistryPassword string `env:"DOCKER_REGISTRY_PASSWORD"`
}

const (
	publicKeyFile  = "pubkey.env"
	privateKeyFile = "privkey.env"
)

func main() {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("failed to parse config: %s\n", err)
	}

	ctx := context.Background()

	// Step 0: Generate or load SSH keys
	log.Println("Setting up SSH keys...")
	publicKey, privateKey, err := ensureSSHKeys()
	if err != nil {
		log.Fatalf("failed to setup SSH keys: %s\n", err)
	}
	log.Println("SSH keys ready")

	// Step 1: Build the Docker image
	log.Println("Building Docker image for example app...")
	imageName := fmt.Sprintf("%s/zeitwork/zeitwork-example-image:latest", cfg.DockerRegistryURL)
	if err := buildDockerImage(ctx, "./apps/example", imageName); err != nil {
		log.Fatalf("failed to build Docker image: %s\n", err)
	}
	log.Printf("Successfully built image: %s\n", imageName)

	// Step 2: Push the image to registry
	log.Println("Pushing image to registry...")
	if err := pushDockerImage(ctx, imageName, cfg.DockerRegistryUsername, cfg.DockerRegistryPassword); err != nil {
		log.Fatalf("failed to push Docker image: %s\n", err)
	}
	log.Printf("Successfully pushed image: %s\n", imageName)

	// Step 3: Create Hetzner server
	log.Println("Creating Hetzner CX23 server...")
	hcloudClient := hcloud.NewClient(hcloud.WithToken(cfg.HCloudToken))
	server, err := createHetznerServer(ctx, hcloudClient, publicKey)
	if err != nil {
		log.Fatalf("failed to create Hetzner server: %s\n", err)
	}
	log.Printf("Successfully created server: %s (IP: %s)\n", server.Name, server.PublicNet.IPv4.IP.String())

	// Step 4: Wait for server to be ready and install Docker
	log.Println("Waiting for server to be ready...")
	time.Sleep(30 * time.Second) // Wait for server to fully boot

	serverIP := server.PublicNet.IPv4.IP.String()
	log.Printf("Installing Docker on server %s...\n", serverIP)
	if err := installDockerOnServer(serverIP, privateKey); err != nil {
		log.Fatalf("failed to install Docker: %s\n", err)
	}
	log.Println("Successfully installed Docker")

	// Step 5: Pull the image on the server
	log.Println("Pulling image on server...")
	if err := pullImageOnServer(serverIP, imageName, cfg.DockerRegistryUsername, cfg.DockerRegistryPassword, privateKey); err != nil {
		log.Fatalf("failed to pull image on server: %s\n", err)
	}
	log.Printf("Successfully pulled image %s on server\n", imageName)

	// Step 6: Run the container
	log.Println("Starting container on server...")
	if err := runContainerOnServer(serverIP, imageName, privateKey); err != nil {
		log.Fatalf("failed to run container on server: %s\n", err)
	}
	log.Printf("Container started successfully on http://%s:8080\n", serverIP)

	log.Println("✓ All steps completed successfully!")
}

func ensureSSHKeys() (publicKey string, privateKey []byte, err error) {
	// Check if keys already exist
	if _, err := os.Stat(publicKeyFile); err == nil {
		if _, err := os.Stat(privateKeyFile); err == nil {
			// Load existing keys
			pubKeyBytes, err := os.ReadFile(publicKeyFile)
			if err != nil {
				return "", nil, fmt.Errorf("failed to read public key: %w", err)
			}
			privKeyBytes, err := os.ReadFile(privateKeyFile)
			if err != nil {
				return "", nil, fmt.Errorf("failed to read private key: %w", err)
			}
			log.Println("Using existing SSH keys")
			return string(pubKeyBytes), privKeyBytes, nil
		}
	}

	// Generate new SSH key pair
	log.Println("Generating new SSH key pair...")
	pubKey, privKey, err := generateSSHKeyPair()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate SSH keys: %w", err)
	}

	// Save public key
	if err := os.WriteFile(publicKeyFile, []byte(pubKey), 0644); err != nil {
		return "", nil, fmt.Errorf("failed to write public key: %w", err)
	}

	// Save private key
	if err := os.WriteFile(privateKeyFile, privKey, 0600); err != nil {
		return "", nil, fmt.Errorf("failed to write private key: %w", err)
	}

	log.Printf("SSH keys generated and saved to %s and %s\n", publicKeyFile, privateKeyFile)
	return pubKey, privKey, nil
}

func generateSSHKeyPair() (publicKey string, privateKey []byte, err error) {
	// Generate ED25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Convert to SSH format
	sshPublicKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}

	// Format public key
	publicKeyStr := string(ssh.MarshalAuthorizedKey(sshPublicKey))

	// Marshal private key to OpenSSH format
	privateKeyBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	return publicKeyStr, pem.EncodeToMemory(privateKeyBytes), nil
}

func buildDockerImage(ctx context.Context, contextPath, imageName string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	tar, err := archive.TarWithOptions(contextPath, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}
	defer tar.Close()

	buildOptions := types.ImageBuildOptions{
		Tags:       []string{imageName},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Platform:   "linux/amd64",
		Version:    types.BuilderBuildKit, // Use BuildKit for multi-platform support
	}

	buildResponse, err := cli.ImageBuild(ctx, tar, buildOptions)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer buildResponse.Body.Close()

	// Read build output
	var buildResult struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
	}
	decoder := json.NewDecoder(buildResponse.Body)
	for decoder.More() {
		if err := decoder.Decode(&buildResult); err != nil {
			return fmt.Errorf("failed to decode build response: %w", err)
		}
		if buildResult.Stream != "" {
			log.Print(buildResult.Stream)
		}
		if buildResult.Error != "" {
			return fmt.Errorf("build error: %s", buildResult.Error)
		}
	}

	return nil
}

func pushDockerImage(ctx context.Context, imageName, username, password string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	authConfig := registry.AuthConfig{
		Username: username,
		Password: password,
	}
	encodedAuth, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("failed to encode auth: %w", err)
	}
	authStr := base64.URLEncoding.EncodeToString(encodedAuth)

	pushResponse, err := cli.ImagePush(ctx, imageName, image.PushOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}
	defer pushResponse.Close()

	// Read push output
	var pushResult struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	decoder := json.NewDecoder(pushResponse)
	for decoder.More() {
		if err := decoder.Decode(&pushResult); err != nil {
			return fmt.Errorf("failed to decode push response: %w", err)
		}
		if pushResult.Status != "" {
			log.Printf("Push: %s\n", pushResult.Status)
		}
		if pushResult.Error != "" {
			return fmt.Errorf("push error: %s", pushResult.Error)
		}
	}

	return nil
}

func createHetznerServer(ctx context.Context, client *hcloud.Client, publicSSHKey string) (*hcloud.Server, error) {
	// Get or create SSH key
	sshKey, _, err := client.SSHKey.GetByName(ctx, "zeitwork-key")
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH key: %w", err)
	}

	// Create SSH key if it doesn't exist
	if sshKey == nil {
		sshKey, _, err = client.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
			Name:      "zeitwork-key",
			PublicKey: publicSSHKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH key: %w", err)
		}
		log.Println("Created new SSH key")
	}

	// Get server type (CX23)
	serverType, _, err := client.ServerType.GetByName(ctx, "cx23")
	if err != nil {
		return nil, fmt.Errorf("failed to get server type: %w", err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("server type cx23 not found")
	}

	// Get Ubuntu image
	imageObj, _, err := client.Image.GetByNameAndArchitecture(ctx, "ubuntu-24.04", hcloud.ArchitectureX86)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}
	if imageObj == nil {
		return nil, fmt.Errorf("ubuntu-24.04 image not found")
	}

	// Get location (Nuremberg)
	location, _, err := client.Location.GetByName(ctx, "nbg1")
	if err != nil {
		return nil, fmt.Errorf("failed to get location: %w", err)
	}

	// Create server
	serverName := fmt.Sprintf("zeitwork-example-%d", time.Now().Unix())
	result, _, err := client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:       serverName,
		ServerType: serverType,
		Image:      imageObj,
		Location:   location,
		SSHKeys:    []*hcloud.SSHKey{sshKey},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// Wait for server to be created
	if err := client.Action.WaitFor(ctx, result.Action); err != nil {
		return nil, fmt.Errorf("failed to wait for server creation: %w", err)
	}

	return result.Server, nil
}

func installDockerOnServer(serverIP string, privateKey []byte) error {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Try to connect with retry
	var sshClient *ssh.Client
	for i := 0; i < 10; i++ {
		sshClient, err = ssh.Dial("tcp", fmt.Sprintf("%s:22", serverIP), sshConfig)
		if err == nil {
			break
		}
		log.Printf("Waiting for SSH to be ready... (attempt %d/10)\n", i+1)
		time.Sleep(10 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer sshClient.Close()

	// Install Docker using get.docker.com
	commands := []string{
		"curl -fsSL https://get.docker.com -o get-docker.sh",
		"sh get-docker.sh",
		"systemctl enable docker",
		"systemctl start docker",
	}

	for _, cmd := range commands {
		session, err := sshClient.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create SSH session: %w", err)
		}

		output, err := session.CombinedOutput(cmd)
		session.Close()
		if err != nil {
			return fmt.Errorf("failed to execute command '%s': %w\nOutput: %s", cmd, err, string(output))
		}
		log.Printf("Executed: %s\n", cmd)
	}

	return nil
}

func pullImageOnServer(serverIP, imageName, username, password string, privateKey []byte) error {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", serverIP), sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer sshClient.Close()

	// Login to registry
	loginCmd := fmt.Sprintf("docker login %s -u %s -p %s",
		extractRegistryDomain(imageName), username, password)
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	output, err := session.CombinedOutput(loginCmd)
	session.Close()
	if err != nil {
		return fmt.Errorf("failed to login to registry: %w\nOutput: %s", err, string(output))
	}
	log.Println("Logged in to registry")

	// Pull image
	pullCmd := fmt.Sprintf("docker pull %s", imageName)
	session, err = sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	output, err = session.CombinedOutput(pullCmd)
	session.Close()
	if err != nil {
		return fmt.Errorf("failed to pull image: %w\nOutput: %s", err, string(output))
	}
	log.Printf("Successfully pulled image: %s\n", imageName)

	return nil
}

func runContainerOnServer(serverIP, imageName string, privateKey []byte) error {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", serverIP), sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer sshClient.Close()

	// Run container with port mapping
	runCmd := fmt.Sprintf("docker run -d -p 8080:8080 --name zeitwork-example --restart unless-stopped %s", imageName)
	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	output, err := session.CombinedOutput(runCmd)
	session.Close()
	if err != nil {
		return fmt.Errorf("failed to run container: %w\nOutput: %s", err, string(output))
	}
	log.Printf("Container started with ID: %s\n", string(output))

	return nil
}

func extractRegistryDomain(imageName string) string {
	// Extract registry domain from full image name
	// e.g., "registry.example.com/zeitwork/zeitwork-example-image:latest" -> "registry.example.com"
	parts := []rune(imageName)
	for i, c := range parts {
		if c == '/' {
			return string(parts[:i])
		}
	}
	return imageName
}
