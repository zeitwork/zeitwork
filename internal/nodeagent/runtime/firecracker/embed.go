package firecracker

import (
	"embed"
	"text/template"
)

//go:embed templates/configs/*
var configTemplates embed.FS

//go:embed templates/scripts/*
var scriptTemplates embed.FS

// Template names for configs
const (
	DaemonConfigTemplate   = "templates/configs/daemon-config.toml.tmpl"
	RuntimeConfigTemplate  = "templates/configs/runtime-config.json.tmpl"
	CNIConfigTemplate      = "templates/configs/cni-config.json.tmpl"
	SystemdServiceTemplate = "templates/configs/systemd-service.tmpl"
)

// Template names for scripts
const (
	InstallFirecrackerTemplate = "templates/scripts/install-firecracker.sh.tmpl"
	SetupDevmapperTemplate     = "templates/scripts/setup-devmapper.sh.tmpl"
	InstallCNITemplate         = "templates/scripts/install-cni.sh.tmpl"
	CreateRootfsTemplate       = "templates/scripts/create-rootfs.sh.tmpl"
	BuildKernelTemplate        = "templates/scripts/build-kernel.sh.tmpl"
	BuildRootfsTemplate        = "templates/scripts/build-rootfs.sh.tmpl"
)

// GetConfigTemplate returns a parsed template for configuration files
func GetConfigTemplate(name string) (*template.Template, error) {
	return template.ParseFS(configTemplates, name)
}

// GetScriptTemplate returns a parsed template for script files
func GetScriptTemplate(name string) (*template.Template, error) {
	return template.ParseFS(scriptTemplates, name)
}

// Template data structures for configuration files

// DaemonConfigData contains data for daemon configuration template
type DaemonConfigData struct {
	ContainerdRoot   string
	ContainerdState  string
	ContainerdSocket string
	DeviceMapperPool string
	BaseImageSize    string
	SnapshotterRoot  string
	LogLevel         string
}

// RuntimeConfigData contains data for runtime configuration template
type RuntimeConfigData struct {
	FirecrackerBinary string
	KernelImagePath   string
	KernelArgs        string
	RootDrive         string
	CPUTemplate       string
	VCPUs             int32
	MemoryMB          int32
	HTEnabled         bool
	RuncBinary        string
	LogLevel          string
}

// CNIConfigData contains data for CNI configuration template
type CNIConfigData struct {
	NetworkName string
	BridgeName  string
	Subnet      string
	RangeStart  string
	RangeEnd    string
	Gateway     string
}

// SystemdServiceData contains data for systemd service template
type SystemdServiceData struct {
	FirecrackerContainerdBinary string
	ConfigPath                  string
}

// Template data structures for script files

// InstallFirecrackerData contains data for firecracker installation script
type InstallFirecrackerData struct {
	FirecrackerVersion string
	TempDir            string
	FirecrackerPath    string
	JailerPath         string
}

// SetupDevmapperData contains data for device mapper setup script
type SetupDevmapperData struct {
	PoolName     string
	DataPath     string
	MetadataPath string
	DataSize     string
	MetadataSize string
}

// InstallCNIData contains data for CNI installation script
type InstallCNIData struct {
	CNIVersion string
	CNIDir     string
	TempDir    string
}

// CreateRootfsData contains data for rootfs creation script
type CreateRootfsData struct {
	RootfsPath string
	RootfsSize string
	TempMount  string
}

// BuildKernelData contains data for kernel build script
type BuildKernelData struct {
	FirecrackerSourceDir string
	KernelVersion        string
	OutputPath           string
	TempDir              string
}

// BuildRootfsData contains data for rootfs build script using Firecracker tools
type BuildRootfsData struct {
	FirecrackerSourceDir string
	OutputPath           string
	TempDir              string
}
