package main

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// Config holds all environment variables
type Config struct {
	// Docker Registry
	DockerRegistryURL      string `env:"DOCKER_REGISTRY_URL,required"`
	DockerRegistryUsername string `env:"DOCKER_REGISTRY_USERNAME,required"`
	DockerRegistryPassword string `env:"DOCKER_REGISTRY_PASSWORD,required"`

	// SSH Keys
	SSHPublicKey  string `env:"SSH_PUBLIC_KEY,required"`
	SSHPrivateKey string `env:"SSH_PRIVATE_KEY,required"`

	// Hetzner
	HetznerToken string `env:"HETZNER_TOKEN,required"`

	// Edgeproxy
	EdgeproxyDatabaseURL string `env:"EDGEPROXY_DATABASE_URL"`
	EdgeproxyRegionID    string `env:"EDGEPROXY_REGION_ID"`

	// Reconciler
	ReconcilerDatabaseURL            string `env:"RECONCILER_DATABASE_URL"`
	ReconcilerHetznerToken           string `env:"RECONCILER_HETZNER_TOKEN"`
	ReconcilerDockerRegistryURL      string `env:"RECONCILER_DOCKER_REGISTRY_URL"`
	ReconcilerDockerRegistryUsername string `env:"RECONCILER_DOCKER_REGISTRY_USERNAME"`
	ReconcilerDockerRegistryPassword string `env:"RECONCILER_DOCKER_REGISTRY_PASSWORD"`
	ReconcilerSSHPublicKey           string `env:"RECONCILER_SSH_PUBLIC_KEY"`
	ReconcilerSSHPrivateKey          string `env:"RECONCILER_SSH_PRIVATE_KEY"`

	// Builder
	BuilderDatabaseURL      string `env:"BUILDER_DATABASE_URL"`
	BuilderGitHubAppID      string `env:"BUILDER_GITHUB_APP_ID"`
	BuilderGitHubAppKey     string `env:"BUILDER_GITHUB_APP_KEY"`
	BuilderRegistryURL      string `env:"BUILDER_REGISTRY_URL"`
	BuilderRegistryUsername string `env:"BUILDER_REGISTRY_USERNAME"`
	BuilderRegistryPassword string `env:"BUILDER_REGISTRY_PASSWORD"`
	BuilderHetznerToken     string `env:"BUILDER_HETZNER_TOKEN"`
	BuilderSSHPublicKey     string `env:"BUILDER_SSH_PUBLIC_KEY"`
	BuilderSSHPrivateKey    string `env:"BUILDER_SSH_PRIVATE_KEY"`
}

// DeployConfig represents the deploy.yaml structure
type DeployConfig struct {
	Regions map[string]RegionConfig `yaml:"regions"`
}

type RegionConfig struct {
	No      int   `yaml:"no"`
	LB      int   `yaml:"lb"`
	Proxies []int `yaml:"proxies"`
	Manager int   `yaml:"manager"`
}

var (
	logger     *slog.Logger
	cfg        Config
	deployCfg  DeployConfig
	rootCmd    *cobra.Command
	envFile    string
	configFile string
)

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}

func main() {
	rootCmd = &cobra.Command{
		Use:   "zeitwork-cli",
		Short: "Zeitwork deployment CLI",
		Long:  "CLI tool for building and deploying Zeitwork services to Hetzner infrastructure",
	}

	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy services to infrastructure",
		Long:  "Build Docker images, push to registry, and deploy to Hetzner servers",
		RunE:  runDeploy,
	}

	deployCmd.Flags().StringVar(&envFile, "env-file", ".env.prod", "Environment file to load")
	deployCmd.Flags().StringVar(&configFile, "config", "config/deploy.yaml", "Deployment configuration file")
	deployCmd.Flags().StringSlice("services", []string{"builder", "edgeproxy", "reconciler"}, "Services to deploy")

	rootCmd.AddCommand(deployCmd)

	if err := rootCmd.Execute(); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func runDeploy(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	if err := loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	services, _ := cmd.Flags().GetStringSlice("services")
	logger.Info("starting deployment", "services", services)

	// Initialize Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer cli.Close()

	// Build and push images
	imageTag := time.Now().Format("20060102-150405")
	images := make(map[string]string)

	for _, service := range services {
		logger.Info("building image", "service", service)
		fullImageName, err := buildImage(ctx, cli, service, imageTag)
		if err != nil {
			return fmt.Errorf("failed to build %s: %w", service, err)
		}
		images[service] = fullImageName
		logger.Info("image built successfully", "service", service, "image", fullImageName)
	}

	// Push images to registry
	if err := loginRegistry(ctx, cli); err != nil {
		return fmt.Errorf("failed to login to registry: %w", err)
	}

	for service, imageName := range images {
		logger.Info("pushing image", "service", service, "image", imageName)
		if err := pushImage(ctx, cli, imageName); err != nil {
			return fmt.Errorf("failed to push %s: %w", service, err)
		}
		logger.Info("image pushed successfully", "service", service)
	}

	// Deploy to servers
	hetznerClient := hcloud.NewClient(hcloud.WithToken(cfg.HetznerToken))

	for _, service := range services {
		serverIDs := getServerIDsForService(service)
		for _, serverID := range serverIDs {
			logger.Info("deploying to server", "service", service, "server_id", serverID)

			server, _, err := hetznerClient.Server.GetByID(ctx, int64(serverID))
			if err != nil {
				return fmt.Errorf("failed to get server %d: %w", serverID, err)
			}

			if server == nil {
				return fmt.Errorf("server %d not found", serverID)
			}

			serverIP := server.PublicNet.IPv4.IP.String()
			logger.Info("deploying to server", "service", service, "ip", serverIP)

			if err := deployToServer(ctx, service, serverIP, images[service]); err != nil {
				return fmt.Errorf("failed to deploy %s to %s: %w", service, serverIP, err)
			}

			logger.Info("deployed successfully", "service", service, "server", serverIP)
		}
	}

	logger.Info("deployment completed successfully")
	return nil
}

func loadConfig() error {
	// Load environment variables from file
	if err := godotenv.Load(envFile); err != nil {
		return fmt.Errorf("failed to load env file: %w", err)
	}

	// Parse environment variables into config struct
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Load deploy.yaml
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &deployCfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

func buildImage(ctx context.Context, cli *client.Client, service, tag string) (string, error) {
	// Create build context
	dockerfile := filepath.Join("docker", service, "Dockerfile")
	buildContext, err := createBuildContext(".")
	if err != nil {
		return "", fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildContext.Close()

	// Construct image name
	imageName := fmt.Sprintf("%s/%s", cfg.DockerRegistryURL, service)
	imageWithTag := fmt.Sprintf("%s:%s", imageName, tag)
	imageLatest := fmt.Sprintf("%s:latest", imageName)

	// Build image for linux/amd64 (server architecture)
	buildOptions := types.ImageBuildOptions{
		Tags:       []string{imageWithTag, imageLatest},
		Dockerfile: dockerfile,
		Remove:     true,
		Platform:   "linux/amd64",
		BuildArgs:  map[string]*string{},
	}

	resp, err := cli.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	// Stream build output and check for errors
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read build output: %w", err)
	}

	// Print output
	fmt.Print(string(output))

	// Check if build was successful by looking for error in JSON output
	if strings.Contains(string(output), `"errorDetail"`) {
		return "", fmt.Errorf("docker build failed - check output above")
	}

	return imageWithTag, nil
}

func createBuildContext(contextPath string) (io.ReadCloser, error) {
	// Create a pipe to stream the tar archive
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		tw := tar.NewWriter(pw)
		defer tw.Close()

		// Walk the directory tree and add files to tar
		err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip common directories that shouldn't be in build context
			relPath, err := filepath.Rel(contextPath, path)
			if err != nil {
				return err
			}

			// Skip unwanted paths
			skipPaths := []string{
				"node_modules",
				".git",
				".next",
				".output",
				"_archive",
				"_archive2",
			}

			for _, skip := range skipPaths {
				if strings.Contains(relPath, skip) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			// Create tar header
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			// Update the name to be relative
			header.Name = relPath

			// Write header
			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// If not a dir, write file content
			if !info.IsDir() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				if _, err := io.Copy(tw, file); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			logger.Error("failed to create tar archive", "error", err)
		}
	}()

	return pr, nil
}

func loginRegistry(ctx context.Context, cli *client.Client) error {
	authConfig := registry.AuthConfig{
		Username:      cfg.DockerRegistryUsername,
		Password:      cfg.DockerRegistryPassword,
		ServerAddress: cfg.DockerRegistryURL,
	}

	_, err := cli.RegistryLogin(ctx, authConfig)
	return err
}

func pushImage(ctx context.Context, cli *client.Client, imageName string) error {
	authConfig := registry.AuthConfig{
		Username:      cfg.DockerRegistryUsername,
		Password:      cfg.DockerRegistryPassword,
		ServerAddress: cfg.DockerRegistryURL,
	}

	encodedAuth, err := encodeAuthConfig(authConfig)
	if err != nil {
		return fmt.Errorf("failed to encode auth: %w", err)
	}

	resp, err := cli.ImagePush(ctx, imageName, image.PushOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}
	defer resp.Close()

	// Stream push output and check for errors
	output, err := io.ReadAll(resp)
	if err != nil {
		return fmt.Errorf("failed to read push output: %w", err)
	}

	// Print output
	fmt.Print(string(output))

	// Check if push was successful
	if strings.Contains(string(output), `"errorDetail"`) {
		return fmt.Errorf("docker push failed - check output above")
	}

	return nil
}

func encodeAuthConfig(authConfig registry.AuthConfig) (string, error) {
	authJSON := fmt.Sprintf(`{"username":"%s","password":"%s","serveraddress":"%s"}`,
		authConfig.Username, authConfig.Password, authConfig.ServerAddress)
	return base64.URLEncoding.EncodeToString([]byte(authJSON)), nil
}

func getServerIDsForService(service string) []int {
	// Get the first region (nbg1)
	var region RegionConfig
	for _, r := range deployCfg.Regions {
		region = r
		break
	}

	switch service {
	case "edgeproxy":
		return region.Proxies
	case "reconciler":
		return []int{region.Manager}
	case "builder":
		return []int{region.Manager}
	default:
		return []int{}
	}
}

func deployToServer(ctx context.Context, service, serverIP, imageName string) error {
	// Create SSH client
	sshClient, err := createSSHClient(serverIP)
	if err != nil {
		return fmt.Errorf("failed to create ssh client: %w", err)
	}
	defer sshClient.Close()

	// Check Docker daemon
	if err := runSSHCommand(sshClient, "docker ps"); err != nil {
		return fmt.Errorf("docker daemon not running: %w", err)
	}

	// Create environment file on remote server
	envFilePath := fmt.Sprintf("/tmp/%s.env", service)
	if err := createRemoteEnvFile(sshClient, service, envFilePath); err != nil {
		return fmt.Errorf("failed to create env file: %w", err)
	}

	// Login to registry on remote server
	loginCmd := fmt.Sprintf("echo '%s' | docker login %s -u %s --password-stdin",
		cfg.DockerRegistryPassword, cfg.DockerRegistryURL, cfg.DockerRegistryUsername)
	if err := runSSHCommand(sshClient, loginCmd); err != nil {
		return fmt.Errorf("failed to login to registry: %w", err)
	}

	// Stop and remove existing container (ignore errors if not exists)
	stopCmd := fmt.Sprintf("docker stop %s 2>/dev/null || true", service)
	runSSHCommand(sshClient, stopCmd)
	removeCmd := fmt.Sprintf("docker rm %s 2>/dev/null || true", service)
	runSSHCommand(sshClient, removeCmd)

	// Pull new image
	pullCmd := fmt.Sprintf("docker pull %s", imageName)
	if err := runSSHCommand(sshClient, pullCmd); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Build docker run command
	runCmd := buildDockerRunCommand(service, imageName, envFilePath)
	if err := runSSHCommand(sshClient, runCmd); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Clean up old images
	cleanupCmd := "docker image prune -f"
	runSSHCommand(sshClient, cleanupCmd)

	return nil
}

func createSSHClient(host string) (*ssh.Client, error) {
	// Handle private key with escaped newlines (from env files)
	privateKey := cfg.SSHPrivateKey
	if strings.Contains(privateKey, `\n`) {
		privateKey = strings.ReplaceAll(privateKey, `\n`, "\n")
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	return client, nil
}

func runSSHCommand(client *ssh.Client, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Capture output
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	logger.Debug("running ssh command", "cmd", cmd)
	return session.Run(cmd)
}

func createRemoteEnvFile(client *ssh.Client, service, remotePath string) error {
	var envContent strings.Builder

	switch service {
	case "edgeproxy":
		if cfg.EdgeproxyDatabaseURL != "" {
			envContent.WriteString(fmt.Sprintf("EDGEPROXY_DATABASE_URL=%s\n", cfg.EdgeproxyDatabaseURL))
		}
		if cfg.EdgeproxyRegionID != "" {
			envContent.WriteString(fmt.Sprintf("EDGEPROXY_REGION_ID=%s\n", cfg.EdgeproxyRegionID))
		}
	case "reconciler":
		if cfg.ReconcilerDatabaseURL != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_DATABASE_URL=%s\n", cfg.ReconcilerDatabaseURL))
		}
		if cfg.ReconcilerHetznerToken != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_HETZNER_TOKEN=%s\n", cfg.ReconcilerHetznerToken))
		}
		if cfg.ReconcilerDockerRegistryURL != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_DOCKER_REGISTRY_URL=%s\n", cfg.ReconcilerDockerRegistryURL))
		}
		if cfg.ReconcilerDockerRegistryUsername != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_DOCKER_REGISTRY_USERNAME=%s\n", cfg.ReconcilerDockerRegistryUsername))
		}
		if cfg.ReconcilerDockerRegistryPassword != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_DOCKER_REGISTRY_PASSWORD=%s\n", cfg.ReconcilerDockerRegistryPassword))
		}
		if cfg.ReconcilerSSHPublicKey != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_SSH_PUBLIC_KEY=%s\n", cfg.ReconcilerSSHPublicKey))
		}
		if cfg.ReconcilerSSHPrivateKey != "" {
			envContent.WriteString(fmt.Sprintf("RECONCILER_SSH_PRIVATE_KEY=%s\n", cfg.ReconcilerSSHPrivateKey))
		}
	case "builder":
		if cfg.BuilderDatabaseURL != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_DATABASE_URL=%s\n", cfg.BuilderDatabaseURL))
		}
		if cfg.BuilderGitHubAppID != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_GITHUB_APP_ID=%s\n", cfg.BuilderGitHubAppID))
		}
		if cfg.BuilderGitHubAppKey != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_GITHUB_APP_KEY=%s\n", cfg.BuilderGitHubAppKey))
		}
		if cfg.BuilderRegistryURL != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_REGISTRY_URL=%s\n", cfg.BuilderRegistryURL))
		}
		if cfg.BuilderRegistryUsername != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_REGISTRY_USERNAME=%s\n", cfg.BuilderRegistryUsername))
		}
		if cfg.BuilderRegistryPassword != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_REGISTRY_PASSWORD=%s\n", cfg.BuilderRegistryPassword))
		}
		if cfg.BuilderHetznerToken != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_HETZNER_TOKEN=%s\n", cfg.BuilderHetznerToken))
		}
		if cfg.BuilderSSHPublicKey != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_SSH_PUBLIC_KEY=%s\n", cfg.BuilderSSHPublicKey))
		}
		if cfg.BuilderSSHPrivateKey != "" {
			envContent.WriteString(fmt.Sprintf("BUILDER_SSH_PRIVATE_KEY=%s\n", cfg.BuilderSSHPrivateKey))
		}
	}

	// Create the file using cat
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", remotePath, envContent.String())
	return runSSHCommand(client, cmd)
}

func buildDockerRunCommand(service, imageName, envFilePath string) string {
	cmd := []string{
		"docker run",
		"-d",
		fmt.Sprintf("--name %s", service),
		"--restart unless-stopped",
		fmt.Sprintf("--env-file %s", envFilePath),
	}

	// Add port mappings based on service
	switch service {
	case "edgeproxy":
		cmd = append(cmd, "-p 8080:8080", "-p 8443:8443")
	case "builder":
		cmd = append(cmd, "-p 8080:8080")
		// Mount docker socket for building images
		cmd = append(cmd, "-v /var/run/docker.sock:/var/run/docker.sock")
	case "reconciler":
		// No exposed ports
	}

	cmd = append(cmd, imageName)

	return strings.Join(cmd, " ")
}
