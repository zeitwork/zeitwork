package config

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration
type Config struct {
	CurrentContext string              `yaml:"current-context"`
	Contexts       []Context           `yaml:"contexts"`
	Clusters       map[string]*Cluster `yaml:"clusters"`
	Users          map[string]*User    `yaml:"users"`
}

// Context represents a cluster context
type Context struct {
	Name    string `yaml:"name"`
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
	EnvFile string `yaml:"env-file"` // e.g., ~/.zeitwork/.env.production
}

// Cluster represents a cluster configuration
type Cluster struct {
	Region        string     `yaml:"region"`
	Operators     []Operator `yaml:"operators"`
	LoadBalancers []Service  `yaml:"load-balancers,omitempty"`
	EdgeProxies   []Service  `yaml:"edge-proxies,omitempty"`
	Nodes         []Node     `yaml:"nodes"`
}

// Operator represents an operator service
type Operator struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Primary bool   `yaml:"primary"`
}

// Service represents a generic service (load balancer, edge proxy)
type Service struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Node represents a worker node
type Node struct {
	Host   string            `yaml:"host"`
	Port   int               `yaml:"port"`
	Region string            `yaml:"region,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// User represents user authentication configuration
type User struct {
	SSHKey     string   `yaml:"ssh-key"`
	SSHUser    string   `yaml:"ssh-user"`
	SSHOptions string   `yaml:"ssh-options,omitempty"`
	Bastion    *Bastion `yaml:"bastion,omitempty"`
}

// Bastion represents bastion/jump host configuration
type Bastion struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Key  string `yaml:"key,omitempty"`
}

// EnvVars holds environment variables loaded from .env file
type EnvVars map[string]string

// LoadConfig loads the configuration from the default location
func LoadConfig() (*Config, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".zeitwork", "config.yaml")
	return LoadConfigFromFile(configPath)
}

// LoadConfigFromFile loads configuration from a specific file
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the configuration to the default location
func (c *Config) SaveConfig() error {
	configPath := filepath.Join(os.Getenv("HOME"), ".zeitwork", "config.yaml")
	return c.SaveConfigToFile(configPath)
}

// SaveConfigToFile saves configuration to a specific file
func (c *Config) SaveConfigToFile(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCurrentContext returns the current context
func (c *Config) GetCurrentContext() (*Context, error) {
	for _, ctx := range c.Contexts {
		if ctx.Name == c.CurrentContext {
			return &ctx, nil
		}
	}
	return nil, fmt.Errorf("current context '%s' not found", c.CurrentContext)
}

// GetContext returns a specific context by name
func (c *Config) GetContext(name string) (*Context, error) {
	for _, ctx := range c.Contexts {
		if ctx.Name == name {
			return &ctx, nil
		}
	}
	return nil, fmt.Errorf("context '%s' not found", name)
}

// GetCluster returns a cluster by name
func (c *Config) GetCluster(name string) (*Cluster, error) {
	cluster, ok := c.Clusters[name]
	if !ok {
		return nil, fmt.Errorf("cluster '%s' not found", name)
	}
	return cluster, nil
}

// GetUser returns a user by name
func (c *Config) GetUser(name string) (*User, error) {
	user, ok := c.Users[name]
	if !ok {
		return nil, fmt.Errorf("user '%s' not found", name)
	}
	return user, nil
}

// LoadEnvFile loads environment variables from a .env file
func LoadEnvFile(path string) (EnvVars, error) {
	// Expand home directory if needed
	if strings.HasPrefix(path, "~/") {
		home := os.Getenv("HOME")
		path = filepath.Join(home, path[2:])
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	env := make(EnvVars)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"'")

		env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}

	return env, nil
}

// GetPrimaryOperator returns the primary operator for a cluster
func (cluster *Cluster) GetPrimaryOperator() *Operator {
	for _, op := range cluster.Operators {
		if op.Primary {
			return &op
		}
	}
	// If no primary designated, return first operator
	if len(cluster.Operators) > 0 {
		return &cluster.Operators[0]
	}
	return nil
}

// GetOperatorURL returns the URL for an operator
func (op *Operator) GetURL() string {
	return fmt.Sprintf("http://%s:%d", op.Host, op.Port)
}

// GetNodeAddress returns the address for a node
func (n *Node) GetAddress() string {
	return fmt.Sprintf("%s:%d", n.Host, n.Port)
}

// SetCurrentContext sets the current context
func (c *Config) SetCurrentContext(name string) error {
	// Verify context exists
	if _, err := c.GetContext(name); err != nil {
		return err
	}
	c.CurrentContext = name
	return nil
}

// InitDefaultConfig creates a default configuration
func InitDefaultConfig() *Config {
	return &Config{
		CurrentContext: "local",
		Contexts: []Context{
			{
				Name:    "local",
				Cluster: "local-cluster",
				User:    "local-admin",
				EnvFile: "~/.zeitwork/.env.local",
			},
		},
		Clusters: map[string]*Cluster{
			"local-cluster": {
				Region: "local",
				Operators: []Operator{
					{
						Host:    "localhost",
						Port:    8080,
						Primary: true,
					},
				},
				Nodes: []Node{
					{
						Host: "localhost",
						Port: 8081,
					},
				},
			},
		},
		Users: map[string]*User{
			"local-admin": {
				SSHKey:  "~/.ssh/id_rsa",
				SSHUser: "root",
			},
		},
	}
}
