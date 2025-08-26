They know the agent is responsible for a lot of things. It is a binary with all dependencies embedded. It is responsible for when initially booting up, making sure the node is properly configured, has KVM support, and is ready to start Firecracker VM instances. It will send health checks to the operators. And it is also responsible for managing resources.

also runs a node-proxy:

Proxy logic:

1. Accept TLS connection
2. Validate client certificate (is request from a valid edge proxy?)
3. Extract target VM from header/SNI (e.g. "vm-1234567890.eu-central-1.zeitwork-dns.com")
4. Check if VM exists locally
5. Proxy to local VM's IPv6:{configured port}
6. Add authentication headers for VM

IPv4 rules:

- Allow 8443 from edge proxy IPs only
- Drop all other inbound traffic

IPv6 rules (internal):

- VMs can't access worker services
- VMs isolated from each other
