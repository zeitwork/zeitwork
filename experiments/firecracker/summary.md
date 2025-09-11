### Firecracker + containerd + IPv6: Findings and guidance for production

This document summarizes what was required to reliably run a Docker-built service inside a Firecracker microVM managed by containerd, reachable over a unique IPv6 from the host.

### Working baseline

- **Host prerequisites**
  - Linux with KVM: `/dev/kvm` present and rw for the user (or run via sudo).
  - Kernel modules: `vhost_vsock`, `tun` loaded.
  - Docker and make/curl/git, dmsetup, jq, iproute2.

- **Core components**
  - firecracker-containerd built from source.
  - Firecracker VMM built (musl) and installed to `/usr/local/bin/firecracker`.
  - Rootfs and kernel placed under `/var/lib/firecracker-containerd/runtime/`.
  - firecracker-containerd config at `/etc/firecracker-containerd/config.toml` with devmapper snapshotter.
  - Runtime JSON at `/etc/containerd/firecracker-runtime.json` with:
    - `default_network_interfaces` pointing to `fcnet6` (CNI),
    - `debug: true`, `shim_base_dir`, and CPU template (we used `C3`).

- **Snapshotter (devmapper) for test/dev**
  - Loopback thinpool created in `/var/lib/firecracker-containerd/snapshotter/devmapper`.
  - Note: loopback is slow and not for production; use a real thinpool/LVM in prod.

- **CNI and IPv6**
  - Installed CNI plugins including `ptp`, `host-local`, `tc-redirect-tap`.
  - IPv6-only network `fcnet6` added to both `/etc/cni/net.d/` and `/etc/cni/conf.d/`:
    - Subnet `fd00:fc::/64`, default route `::/0`.
  - firecracker-containerd runtime uses this by default when creating the VM.

### Why it wasn’t working initially (and how we fixed it)

1. **VM agent crashed due to GLIBC mismatch → vsock handshake timeouts**
   - Symptom: `VM didn't start within 20s: failed to dial the VM over vsock`. Logs showed inside-VM agent errors: missing `GLIBC_2.32`/`2.34`.
   - Root cause: The agent binary was dynamically linked against a newer glibc than the Debian bullseye rootfs provided.
   - Fix:
     - Rebuild the agent statically: `STATIC_AGENT=on CGO_ENABLED=0 make agent`.
     - Regenerate rootfs (`make image`), install to runtime, and use the debug rootfs variant when diagnosing.

2. **CNI config not discovered**
   - Symptom: `failed to load CNI configuration... no net configuration with name "fcnet6"`.
   - Root cause: Some setups read `/etc/cni/conf.d` instead of `/etc/cni/net.d`.
   - Fix: Place `fcnet6.conflist` in both locations.

3. **No IPv6 assigned inside the VM**
   - Symptom: `eth0` existed but had no address/routes; only `lo` had `::1`.
   - Root cause: With `ptp` + `tc-redirect-tap`, the host allocates a lease (`/var/lib/cni/networks/fcnet6/…`), but the in-VM interface isn’t auto-configured.
   - Fix:
     - Discover the lease by matching the VM ID in `/var/lib/cni/networks/fcnet6/`.
     - Exec into the task to set link up, add IPv6, and add a default route:
       ```bash
       ip link set eth0 up
       ip -6 addr add fd00:fc::<N>/64 dev eth0
       ip -6 route add default via fd00:fc::1 dev eth0
       ```
     - The task needs capability to modify links; we added `--cap-add CAP_NET_ADMIN` to `firecracker-ctr run`.

4. **Docker buildx permission error**
   - Symptom: `~/.docker/buildx/.lock: permission denied`.
   - Fix: run `docker build/save` with `sudo` (or add user to docker group with a new login session).

5. **Stale containerd socket and logging quirks**
   - Fix: on daemon start, remove stale socket if daemon isn’t running; log to `/tmp/firecracker-containerd.log` and wait for socket readiness.

### Networking/IPv6 details

- CNI `fcnet6`:
  - `ptp` creates a veth pair; `tc-redirect-tap` turns it into a tap device for FC.
  - Host IPAM writes leases under `/var/lib/cni/networks/fcnet6/` and configures a host-side veth with `fd00:fc::1/128` acting as a gateway.
  - Inside the VM, manually configure `eth0` with the leased address and `fd00:fc::1` as default gateway.
  - The service must listen on IPv6 (we used busybox httpd with `-p [::]:3000`).

### Runbook (what the scripts do)

In `experiments/firecracker/`:

- `run_all.sh` executes:
  - `10_prereqs.sh`: install tools, verify KVM.
  - `15_kernel_mods.sh`: load `vhost_vsock` and `tun`.
  - `20_build_firecracker_containerd.sh`: build static agent, rootfs, firecracker.
  - `30_devmapper_thinpool.sh`: create loopback thinpool.
  - `40_write_configs.sh`: write containerd and runtime configs (IPv6 CNI, debug on).
  - `42_use_debug_rootfs.sh`: copy debug rootfs in place (aids diagnostics).
  - `45_cni_ipv6.sh`: write `fcnet6` conflist to both CNI dirs.
  - `50_start_daemon.sh`: start `firecracker-containerd` and wait for socket.
  - `60_build_image_import.sh`: build `local/hello3000` and import into namespace.
  - `70_run_container_ipv6.sh`: run VM-backed container (adds `CAP_NET_ADMIN`).
  - `75_set_ipv6_inside.sh`: discover lease and configure IPv6 inside the VM.
  - `80_verify_ipv6.sh`: curl `http://[<ipv6>]:3000` from the host; expect `hello world`.

### Production recommendations

- **Agent and rootfs**
  - Prefer a consistent distro baseline (rootfs and agent built against the same glibc) or keep the agent fully static.
  - Pin versions of Firecracker, containerd, and firecracker-containerd.

- **Networking**
  - Avoid manual IP assignment inside the VM; wire up the agent/boot sequence to read and apply the lease automatically (or use a CNI approach that configures the guest link).
  - Keep `fcnet6` (or equivalent) in a dedicated IPv6 ULA prefix per environment. Enforce isolation with firewall rules.
  - Ensure DNS resolvers are not localhost-only on the host (or specify explicit DNS in CNI config).

- **Storage**
  - Replace loopback thinpool with a real device-mapper thinpool or other production-grade snapshotter backend.

- **Security & isolation**
  - Use Firecracker `jailer` instead of noop, sandboxes per workload, minimal capabilities (drop `CAP_NET_ADMIN` if the guest is auto-configured), and lock down host namespaces and mounts. Audit cgroup and seccomp profiles.

- **Observability**
  - Persist logs and metrics (`log_fifo`, `metrics_fifo`) and aggregate with your logging pipeline. Add health checks to detect guest network misconfiguration early.

### Key takeaways

- Vsock startup failures often trace back to the agent binary not matching the rootfs libc; a static agent or aligned rootfs fixes this class of issues.
- The `tc-redirect-tap` path provides a tap to the guest but does not by itself configure addresses inside the VM; you must apply the lease inside the guest (or automate that in the guest init/agent).
- CNI config discovery paths can differ across environments; writing to both `/etc/cni/net.d` and `/etc/cni/conf.d` is pragmatic.
- Development shortcuts (loopback thinpool, sudo docker) must be replaced by hardened equivalents for production.
