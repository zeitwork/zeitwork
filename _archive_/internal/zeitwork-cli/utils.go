package cli

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// generateSSHKeyPair generates a new SSH key pair
func generateSSHKeyPair() (string, error) {
	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", err
	}

	// Create .deploy directory if it doesn't exist
	deployDir := filepath.Join(".", ".deploy")
	if err := os.MkdirAll(deployDir, 0700); err != nil {
		return "", err
	}

	// Save private key
	privateKeyPath := filepath.Join(deployDir, "id_rsa")
	privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer privateKeyFile.Close()

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return "", err
	}

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", err
	}

	// Save public key
	publicKeyPath := filepath.Join(deployDir, "id_rsa.pub")
	publicKeyData := ssh.MarshalAuthorizedKey(publicKey)
	if err := os.WriteFile(publicKeyPath, publicKeyData, 0644); err != nil {
		return "", err
	}

	return privateKeyPath, nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err == nil {
			path = filepath.Join(usr.HomeDir, path[2:])
		}
	}
	return path
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(expandPath(path))
	return err == nil
}

// findProjectRoot finds the project root directory (where go.mod is)
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("project root not found")
		}
		dir = parent
	}
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(path string) (map[string]string, error) {
	envMap := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first = sign
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Remove surrounding quotes if present
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}

			envMap[key] = value
		}
	}

	return envMap, scanner.Err()
}

// parseIPList parses a comma-separated list of IP addresses
func parseIPList(ipList string) []string {
	if ipList == "" {
		return []string{}
	}

	var ips []string
	for _, ip := range strings.Split(ipList, ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips
}
