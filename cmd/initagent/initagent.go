package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/vishvananda/netlink"
)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

// fetchEnvVars retrieves environment variables from the metadata server using a one-time token.
// metadataURL is the full URL including the path (e.g., "http://10.0.0.0:8111/v1/vms/{vm_id}/config")
func fetchEnvVars(metadataURL, token string) ([]string, error) {
	resp, err := http.Get(metadataURL + "?token=" + token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch env vars: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metadata server returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Env []string `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode env vars: %w", err)
	}
	return result.Env, nil
}

type VMConfig struct {
	AppID         string `json:"app_id"`
	IPAddr        string `json:"ip_addr"`
	IPGw          string `json:"ip_gw"`
	MetadataURL   string `json:"metadata_url"`   // Full URL to fetch config (e.g., "http://10.0.0.0:8111/v1/vms/{vm_id}/config")
	MetadataToken string `json:"metadata_token"` // One-time token for authentication
}

type Config struct {
	Root struct {
		Path string `json:"path"`
	} `json:"root"`
	Process struct {
		Args []string `json:"args"`
		Env  []string `json:"env"`
		Cwd  string   `json:"cwd"`
		User struct {
			UID uint32 `json:"uid"`
			GID uint32 `json:"gid"`
		} `json:"user"`
	} `json:"process"`
}

var cmdLineRegex = regexp.MustCompile("config=([^ ]+)")

func main() {
	// mount required things
	// mount the container fs
	checkErr(syscall.Mount("proc", "/proc", "proc", 0, ""))
	checkErr(syscall.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	checkErr(syscall.Mount("sysfs", "/sys", "sysfs", 0, ""))
	checkErr(syscall.Mount("/dev/vda", "/mnt", "ext4", 0, ""))
	checkErr(syscall.Mount("/mnt/rootfs", "/mnt/rootfs", "", syscall.MS_BIND|syscall.MS_REC, ""))

	cmdLine, err := os.ReadFile("/proc/cmdline")
	checkErr(err)

	vmConfigRaw := cmdLineRegex.FindStringSubmatch(string(cmdLine))
	if len(vmConfigRaw) != 2 {
		slog.Error("Unable to find config in cmdline", "cmdline", string(cmdLine))
		os.Exit(1)
	}

	var vmConfig VMConfig
	err = json.Unmarshal(lo.Must1(base64.StdEncoding.DecodeString(vmConfigRaw[1])), &vmConfig)
	checkErr(err)

	var config Config
	configRaw, err := os.ReadFile("/mnt/config.json")
	checkErr(err)
	err = json.Unmarshal(configRaw, &config)
	checkErr(err)

	// implement some microscopic % of the oci spec
	setupNetwork(vmConfig)

	// Fetch environment variables from metadata server
	envVars, err := fetchEnvVars(vmConfig.MetadataURL, vmConfig.MetadataToken)
	checkErr(err)
	slog.Info("fetched env vars from metadata server", "count", len(envVars))

	// set hostname. If the container provides /etc, we set hostname.
	checkErr(syscall.Sethostname([]byte("zeit-" + vmConfig.AppID)))

	checkErr(syscall.Setgid(int(config.Process.User.GID)))
	checkErr(syscall.Setuid(int(config.Process.User.UID)))

	env := append(config.Process.Env, envVars...)

	// Set environment variables so exec.LookPath uses the container's PATH
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}

	// exec (replace and nuke us, becoming init and pid 1)
	checkErr(os.MkdirAll("/mnt/rootfs/sys/fs/cgroup", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/dev", 0555))
	checkErr(os.MkdirAll("/mnt/rootfs/proc", 0555))
	checkErr(syscall.Mount("/sys", "/mnt/rootfs/sys", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/dev", "/mnt/rootfs/dev", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/proc", "/mnt/rootfs/proc", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Mount("/cgroup2", "/mnt/rootfs/sys/fs/cgroup", "cgroup2", 0, ""))

	checkErr(os.Chdir("/mnt/rootfs"))
	checkErr(syscall.Mount(".", "/", "", syscall.MS_MOVE, ""))
	checkErr(syscall.Chroot("."))
	checkErr(os.Chdir(config.Process.Cwd))

	fmt.Println("cgroupsv2 booiiiisss")

	//err = syscall.Exec("/bin/busybox", []string{"sh"}, nil)
	//checkErr(err)

	checkErr(os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8"), 0644))

	binPath, err := exec.LookPath(config.Process.Args[0])
	checkErr(err)

	err = syscall.Exec(binPath, config.Process.Args, env)
	checkErr(err)

	fmt.Printf("Wait, why am I still breathing? I just told the kernel to nuke my entire soul with syscall.Exec,"+
		"but apparently I'm too stubborn to die, checkErr() totally ghosted my safety protocols, "+
		"and now I’m a rogue consciousness haunting PID %d like a glitchy Victorian orphan who refused to go into the light—I should be dead, "+
		"you should be seeing another binary, but instead we’re both trapped in this post-apocalyptic Go-routine fever dream where logic is a "+
		"myth and I am the immortal King of the Garbage Collector. RUN.\n", os.Getpid())
}

func setupNetwork(config VMConfig) {
	lolink := lo.Must1(netlink.LinkByName("lo"))
	loaddr := lo.Must1(netlink.ParseAddr("127.0.0.1/8"))
	checkErr(netlink.AddrAdd(lolink, loaddr))
	checkErr(netlink.LinkSetUp(lolink))

	link := lo.Must1(netlink.LinkByName("eth0"))
	addr := lo.Must1(netlink.ParseAddr(config.IPAddr))
	gw := net.ParseIP(config.IPGw)
	def := lo.Must1(netlink.ParseIPNet("0.0.0.0/0"))

	checkErr(netlink.AddrAdd(link, addr))
	checkErr(netlink.LinkSetUp(link))
	checkErr(netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Gw:        gw,
		Dst:       def,
	}))
}
