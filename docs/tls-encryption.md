# TLS Encryption for Internal Communication

The platform implements mutual TLS (mTLS) authentication for secure communication between internal services, ensuring all traffic between edge proxies, load balancers, and worker nodes is encrypted and authenticated.

## Architecture

```
┌────────────────────┐
│   External User    │
└─────────┬──────────┘
          │ TCP Port 443/80
          ↓
┌────────────────────┐
│   L4 Load Balancer │ ← TCP Distribution
├────────────────────┤
│  • Round Robin     │
│  • Health Checks   │
│  • TLS Passthrough │
└─────────┬──────────┘
          │ TCP/TLS
          ↓
┌────────────────────┐
│  L7 Edge Proxy(s)  │ ← External TLS (Let's Encrypt)
├────────────────────┤
│  • TLS Termination │
│  • Domain Routing  │
│  • Rate Limiting   │
│  • mTLS Client     │ ← Internal mTLS Client
└─────────┬──────────┘
          │ mTLS
          ↓
┌────────────────────┐
│    Worker Node     │ ← Internal mTLS Server
├────────────────────┤
│  • Server Cert     │
│  • Client Verify   │
│  • VM Instances    │
└────────────────────┘
```

## Features

### 1. Internal Certificate Authority

Self-managed CA for internal certificates:

- **10-year CA certificate** for long-term stability
- **90-day service certificates** with automatic rotation
- **RSA 4096-bit** CA key for strong security
- **RSA 2048-bit** service keys for performance

### 2. Mutual TLS (mTLS)

Both client and server authenticate each other:

- **Client certificates** identify the calling service
- **Server certificates** authenticate the target service
- **Certificate validation** on both ends
- **Common Name (CN) verification** for service identity

### 3. Certificate Management

Automated certificate lifecycle:

- **Automatic generation** on service startup
- **Certificate caching** for performance
- **Rotation before expiry** (30-day warning)
- **Persistent storage** for disaster recovery

### 4. TLS Configuration

Strong security defaults:

- **TLS 1.3 minimum** for internal traffic
- **Strong cipher suites** (AES-256-GCM, ChaCha20-Poly1305)
- **Perfect Forward Secrecy** with ECDHE
- **No weak protocols** or cipher suites

## Implementation

### Edge Proxy

```go
// Enable mTLS for internal communication
config := &Config{
    EnableMTLS:      true,
    InternalCAPath:  "/var/lib/zeitwork/ca/ca.crt",
    InternalKeyPath: "/var/lib/zeitwork/ca/ca.key",
    InternalCertDir: "/var/lib/zeitwork/certs",
}
```

The edge proxy:

1. Uses Let's Encrypt certificates for external traffic
2. Generates client certificate for internal requests
3. Validates load balancer's server certificate
4. Upgrades HTTP to HTTPS for internal calls

### Load Balancer

```go
// Accept mTLS connections from edge proxy
if config.EnableMTLS {
    // Create TLS listener with server certificate
    listener = CreateTLSListener(port, tlsConfig)
    // Verify edge proxy client certificate
    tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
}
```

The load balancer:

1. Presents server certificate to edge proxy
2. Validates edge proxy's client certificate
3. Uses client certificates for worker connections
4. Maintains secure connection pools

### Worker Nodes

Worker nodes accept mTLS connections:

1. Present server certificate
2. Validate client certificates
3. Extract client identity from CN
4. Route to appropriate VM instance

## Configuration

### Environment Variables

```bash
# Edge Proxy
ENABLE_MTLS=true
INTERNAL_CA_PATH=/var/lib/zeitwork/ca/ca.crt
INTERNAL_KEY_PATH=/var/lib/zeitwork/ca/ca.key
INTERNAL_CERT_DIR=/var/lib/zeitwork/certs

# Load Balancer
ENABLE_MTLS=true
INTERNAL_CA_PATH=/var/lib/zeitwork/ca/ca.crt
INTERNAL_KEY_PATH=/var/lib/zeitwork/ca/ca.key
INTERNAL_CERT_DIR=/var/lib/zeitwork/certs
```

### Certificate Paths

```
/var/lib/zeitwork/
├── ca/
│   ├── ca.crt         # CA certificate
│   └── ca.key         # CA private key (mode 0600)
└── certs/
    ├── edge-proxy.crt  # Edge proxy certificate
    ├── edge-proxy.key  # Edge proxy private key
    ├── load-balancer.crt
    ├── load-balancer.key
    ├── node-agent.crt
    └── node-agent.key
```

## Security Features

### Certificate Validation

Each connection validates:

- **Certificate chain** to internal CA
- **Certificate validity** period
- **Common Name** matches expected service
- **Key usage** extensions

### Rotation Strategy

Certificates rotate automatically:

1. **Check daily** for certificates near expiry
2. **Generate new certificate** 30 days before expiry
3. **Graceful transition** with overlapping validity
4. **No downtime** during rotation

### Fallback Mechanism

Graceful degradation if mTLS fails:

1. **Log warning** about TLS failure
2. **Fall back to HTTP** if both sides agree
3. **Alert operators** for investigation
4. **Continue service** with reduced security

## Monitoring

### Health Checks

Monitor TLS health:

```bash
# Check certificate expiry
openssl x509 -in /var/lib/zeitwork/certs/edge-proxy.crt -noout -dates

# Verify certificate chain
openssl verify -CAfile /var/lib/zeitwork/ca/ca.crt \
    /var/lib/zeitwork/certs/edge-proxy.crt

# Test mTLS connection
openssl s_client -connect load-balancer:8082 \
    -cert /var/lib/zeitwork/certs/edge-proxy.crt \
    -key /var/lib/zeitwork/certs/edge-proxy.key \
    -CAfile /var/lib/zeitwork/ca/ca.crt
```

### Metrics

Track TLS metrics:

- Certificate expiry times
- TLS handshake failures
- Certificate validation errors
- Protocol version usage
- Cipher suite distribution

## Troubleshooting

### Common Issues

1. **Certificate expired**

   - Check certificate dates
   - Trigger manual rotation
   - Verify CA certificate is valid

2. **TLS handshake failed**

   - Check both certificates are valid
   - Verify CA is the same on both sides
   - Check network connectivity

3. **Certificate not trusted**

   - Ensure CA certificate is distributed
   - Verify certificate chain
   - Check CN matches service name

4. **Permission denied on key file**
   - Set correct permissions (0600)
   - Check file ownership
   - Verify service user can read

### Manual Certificate Generation

Generate certificates manually if needed:

```bash
# Generate server certificate
./scripts/generate-cert.sh server load-balancer

# Generate client certificate
./scripts/generate-cert.sh client edge-proxy

# Distribute CA certificate
cp /var/lib/zeitwork/ca/ca.crt /etc/zeitwork/ca.crt
```

## Performance Considerations

### Connection Pooling

Reuse TLS connections:

- Keep connections alive for 5 minutes
- Pool size of 100 connections per service
- Lazy connection establishment
- Health check on idle connections

### Session Resumption

Speed up TLS handshakes:

- TLS session tickets enabled
- Session cache for 1000 sessions
- 1-hour session timeout
- Automatic session rotation

### Hardware Acceleration

Utilize hardware when available:

- AES-NI for AES operations
- RDRAND for random generation
- Async crypto operations
- CPU affinity for TLS threads

## Future Enhancements

- [ ] Hardware Security Module (HSM) support
- [ ] Certificate transparency logging
- [ ] OCSP stapling for revocation
- [ ] Automated certificate distribution
- [ ] Zero-downtime CA rotation
- [ ] External PKI integration
