package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	latitudesh "github.com/latitudesh/latitudesh-go-sdk"
	"github.com/latitudesh/latitudesh-go-sdk/models/components"
	"github.com/latitudesh/latitudesh-go-sdk/models/operations"
)

// requiredEnv reads an environment variable or panics if empty.
func requiredEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "error: %s environment variable is required\n", key)
		os.Exit(1)
	}
	return v
}

// newLatitudeClient creates a Latitude.sh SDK client from env vars.
func newLatitudeClient() *latitudesh.Latitudesh {
	apiKey := requiredEnv("LATITUDE_API_KEY")
	return latitudesh.New(latitudesh.WithSecurity(apiKey))
}

// generateSSHKeypair generates an ed25519 SSH keypair at the .e2e/ paths.
func generateSSHKeypair() error {
	if err := os.MkdirAll(e2eDir, 0o755); err != nil {
		return err
	}
	// Remove existing keys if any
	os.Remove(sshKeyFile)
	os.Remove(sshPubKeyFile)

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", sshKeyFile, "-N", "", "-q")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createSSHKey uploads the public key to Latitude and returns the key ID.
func createSSHKey(ctx context.Context, client *latitudesh.Latitudesh, projectID string) (string, error) {
	pubKey, err := os.ReadFile(sshPubKeyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	res, err := client.SSHKeys.Create(ctx, operations.PostSSHKeySSHKeysRequestBody{
		Data: operations.PostSSHKeySSHKeysData{
			Type: operations.PostSSHKeySSHKeysTypeSSHKeys,
			Attributes: &operations.PostSSHKeySSHKeysAttributes{
				Name:      latitudesh.String("zeitwork-e2e"),
				Project:   latitudesh.String(projectID),
				PublicKey: latitudesh.String(string(pubKey)),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create SSH key: %w", err)
	}
	if res.Object == nil || res.Object.Data == nil || res.Object.Data.ID == nil {
		return "", fmt.Errorf("SSH key creation returned no ID")
	}
	return *res.Object.Data.ID, nil
}

// createVLAN creates a VLAN at the specified site and returns (vlanID, vid).
func createVLAN(ctx context.Context, client *latitudesh.Latitudesh, projectID, site string) (string, int64, error) {
	siteEnum, err := parseSiteForVLAN(site)
	if err != nil {
		return "", 0, err
	}

	res, err := client.PrivateNetworks.Create(ctx, operations.CreateVirtualNetworkPrivateNetworksRequestBody{
		Data: operations.CreateVirtualNetworkPrivateNetworksData{
			Type: operations.CreateVirtualNetworkPrivateNetworksTypeVirtualNetwork,
			Attributes: operations.CreateVirtualNetworkPrivateNetworksAttributes{
				Description: "zeitwork-e2e",
				Project:     projectID,
				Site:        siteEnum,
			},
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to create VLAN: %w", err)
	}
	if res.VirtualNetwork == nil || res.VirtualNetwork.Data == nil || res.VirtualNetwork.Data.ID == nil {
		return "", 0, fmt.Errorf("VLAN creation returned no ID")
	}

	vlanID := *res.VirtualNetwork.Data.ID
	vid := int64(0)
	if res.VirtualNetwork.Data.Attributes != nil && res.VirtualNetwork.Data.Attributes.Vid != nil {
		vid = *res.VirtualNetwork.Data.Attributes.Vid
	}
	return vlanID, vid, nil
}

// createServer creates a bare metal server and returns its Latitude ID.
func createServer(ctx context.Context, client *latitudesh.Latitudesh, projectID, site, plan, hostname string, sshKeyID string) (string, error) {
	planEnum, err := parsePlan(plan)
	if err != nil {
		return "", err
	}
	siteEnum, err := parseSiteForServer(site)
	if err != nil {
		return "", err
	}
	osEnum := operations.CreateServerOperatingSystemUbuntu2404X64Lts
	billing := operations.CreateServerBillingHourly

	res, err := client.Servers.Create(ctx, operations.CreateServerServersRequestBody{
		Data: &operations.CreateServerServersData{
			Type: operations.CreateServerServersTypeServers,
			Attributes: &operations.CreateServerServersAttributes{
				Project:         latitudesh.String(projectID),
				Plan:            &planEnum,
				Site:            &siteEnum,
				OperatingSystem: &osEnum,
				Hostname:        latitudesh.String(hostname),
				SSHKeys:         []string{sshKeyID},
				Billing:         &billing,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create server %s: %w", hostname, err)
	}
	if res.Server == nil || res.Server.Data == nil || res.Server.Data.ID == nil {
		return "", fmt.Errorf("server creation returned no ID")
	}
	return *res.Server.Data.ID, nil
}

// waitForServer polls until the server status is "on" (ready). Returns the public IP.
func waitForServer(ctx context.Context, client *latitudesh.Latitudesh, serverID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for server %s to be ready", serverID)
		}

		res, err := client.Servers.Get(ctx, serverID, nil)
		if err != nil {
			slog.Warn("failed to poll server status", "server_id", serverID, "err", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if res.Server != nil && res.Server.Data != nil && res.Server.Data.Attributes != nil {
			attrs := res.Server.Data.Attributes
			status := ""
			if attrs.Status != nil {
				status = string(*attrs.Status)
			}
			slog.Info("server status", "server_id", serverID, "status", status)

			if attrs.Status != nil && *attrs.Status == components.StatusOn {
				ip := ""
				if attrs.PrimaryIpv4 != nil {
					ip = *attrs.PrimaryIpv4
				}
				if ip == "" {
					return "", fmt.Errorf("server %s is ready but has no public IPv4", serverID)
				}
				return ip, nil
			}
		}
		time.Sleep(10 * time.Second)
	}
}

// assignServerToVLAN assigns a server to a VLAN and returns the assignment ID.
func assignServerToVLAN(ctx context.Context, client *latitudesh.Latitudesh, serverID, vlanID string) (string, error) {
	res, err := client.PrivateNetworks.Assign(ctx, operations.AssignServerVirtualNetworkPrivateNetworksRequestBody{
		Data: &operations.AssignServerVirtualNetworkPrivateNetworksData{
			Type: operations.AssignServerVirtualNetworkPrivateNetworksTypeVirtualNetworkAssignment,
			Attributes: &operations.AssignServerVirtualNetworkPrivateNetworksAttributes{
				ServerID:         serverID,
				VirtualNetworkID: vlanID,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to assign server %s to VLAN %s: %w", serverID, vlanID, err)
	}
	if res.VirtualNetworkAssignment == nil || res.VirtualNetworkAssignment.Data == nil || res.VirtualNetworkAssignment.Data.ID == nil {
		return "", fmt.Errorf("VLAN assignment returned no ID")
	}
	return *res.VirtualNetworkAssignment.Data.ID, nil
}

// deleteServer deletes a server from Latitude.
func deleteServer(ctx context.Context, client *latitudesh.Latitudesh, serverID string) error {
	_, err := client.Servers.Delete(ctx, serverID, nil)
	return err
}

// deleteSSHKey deletes an SSH key from Latitude.
func deleteSSHKey(ctx context.Context, client *latitudesh.Latitudesh, keyID string) error {
	_, err := client.SSHKeys.Delete(ctx, keyID)
	return err
}

// deleteVLAN deletes a VLAN from Latitude.
func deleteVLAN(ctx context.Context, client *latitudesh.Latitudesh, vlanID string) error {
	_, err := client.VirtualNetworks.Delete(ctx, vlanID)
	return err
}

// parseSiteForVLAN maps a site string to the VLAN creation enum.
func parseSiteForVLAN(site string) (*operations.CreateVirtualNetworkPrivateNetworksSite, error) {
	m := map[string]operations.CreateVirtualNetworkPrivateNetworksSite{
		"ASH": operations.CreateVirtualNetworkPrivateNetworksSiteAsh,
		"DAL": operations.CreateVirtualNetworkPrivateNetworksSiteDal,
		"CHI": operations.CreateVirtualNetworkPrivateNetworksSiteChi,
		"MIA": operations.CreateVirtualNetworkPrivateNetworksSiteMia,
		"NYC": operations.CreateVirtualNetworkPrivateNetworksSiteNyc,
		"LAX": operations.CreateVirtualNetworkPrivateNetworksSiteLax,
		"SAO": operations.CreateVirtualNetworkPrivateNetworksSiteSao,
		"LON": operations.CreateVirtualNetworkPrivateNetworksSiteLon,
		"FRA": operations.CreateVirtualNetworkPrivateNetworksSiteFra,
		"SGP": operations.CreateVirtualNetworkPrivateNetworksSiteSgp,
		"SYD": operations.CreateVirtualNetworkPrivateNetworksSiteSyd,
		"TYO": operations.CreateVirtualNetworkPrivateNetworksSiteTyo,
	}
	v, ok := m[site]
	if !ok {
		return nil, fmt.Errorf("unsupported site: %s", site)
	}
	return &v, nil
}

// parseSiteForServer maps a site string to the server creation enum.
func parseSiteForServer(site string) (operations.CreateServerSite, error) {
	m := map[string]operations.CreateServerSite{
		"ASH": operations.CreateServerSiteAsh,
		"DAL": operations.CreateServerSiteDal,
		"CHI": operations.CreateServerSiteChi,
		"MIA": operations.CreateServerSiteMia,
		"NYC": operations.CreateServerSiteNyc,
		"LAX": operations.CreateServerSiteLax,
		"SAO": operations.CreateServerSiteSao,
		"LON": operations.CreateServerSiteLon,
		"FRA": operations.CreateServerSiteFra,
		"SGP": operations.CreateServerSiteSgp,
		"SYD": operations.CreateServerSiteSyd,
		"TYO": operations.CreateServerSiteTyo,
	}
	v, ok := m[site]
	if !ok {
		return "", fmt.Errorf("unsupported site: %s", site)
	}
	return v, nil
}

// parsePlan maps a plan string to the server creation enum.
func parsePlan(plan string) (operations.CreateServerPlan, error) {
	m := map[string]operations.CreateServerPlan{
		"c2-small-x86":  operations.CreateServerPlanC2SmallX86,
		"c2-medium-x86": operations.CreateServerPlanC2MediumX86,
		"c2-large-x86":  operations.CreateServerPlanC2LargeX86,
		"c3-small-x86":  operations.CreateServerPlanC3SmallX86,
		"c3-medium-x86": operations.CreateServerPlanC3MediumX86,
		"c3-large-x86":  operations.CreateServerPlanC3LargeX86,
		"c3-xlarge-x86": operations.CreateServerPlanC3XlargeX86,
	}
	v, ok := m[plan]
	if !ok {
		return "", fmt.Errorf("unsupported plan: %s", plan)
	}
	return v, nil
}
