# Internal Module Structure

This directory contains the internal modules and scripts for the Firecracker Manager system.

## Directory Structure

```
internal/
├── builder/          # Image builder module
│   └── scripts/
│       └── build_image.sh    # Main script for building Docker images from GitHub repos
│
├── manager/          # Instance and VM manager module
│   ├── config/
│   │   └── vm_config_template.json    # Firecracker VM configuration template
│   └── scripts/
│       ├── setup_vm.sh               # VM setup script
│       └── setup_kvm.sh              # KVM setup script
│
└── node/            # Node management module
    └── scripts/
        ├── install_all.sh            # Install all dependencies
        ├── install_cni_plugins.sh    # CNI plugins installation
        ├── install_containerd.sh     # Containerd installation
        ├── install_dependencies.sh   # Basic dependencies
        ├── install_firecracker.sh    # Firecracker installation
        ├── install_go.sh             # Go installation
        ├── install_runc.sh           # runC installation
        └── utils.sh                  # Utility functions
```

## Module Descriptions

### Builder Module

Handles building container images from GitHub repositories. The build script:

- Clones GitHub repositories
- Checks for Dockerfile presence
- Builds Docker images
- Exports images for use with Firecracker

### Manager Module

Manages Firecracker instances and VMs:

- VM configuration templates
- VM setup and initialization scripts
- KVM environment setup

### Node Module

Handles node setup and dependency installation:

- Installation scripts for all required components
- Utility functions for common operations
