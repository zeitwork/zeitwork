# Zeitwork Platform Upgrade Guide

This directory contains comprehensive upgrade documentation for all Zeitwork platform components.

## Quick Navigation

- [Operator Upgrade](./operator-upgrade.md) - Central control plane service upgrade procedures
- [Node Agent Upgrade](./node-agent-upgrade.md) - Worker node agent upgrade procedures
- [Load Balancer Upgrade](./load-balancer-upgrade.md) - Load balancer service upgrade procedures
- [Edge Proxy Upgrade](./edge-proxy-upgrade.md) - Edge proxy service upgrade procedures
- [Full Platform Upgrade](./platform-upgrade.md) - Complete platform upgrade orchestration
- [Database Migrations](./database-migrations.md) - Database schema migration procedures
- [Rollback Procedures](./rollback-procedures.md) - Emergency rollback documentation

## Upgrade Philosophy

The Zeitwork platform is designed for zero-downtime upgrades through:

1. **Rolling Updates**: Services are upgraded one at a time while others handle traffic
2. **Backward Compatibility**: New versions maintain compatibility with previous versions
3. **Health Monitoring**: Continuous health checks during upgrades
4. **Automatic Rollback**: Failed upgrades trigger automatic rollback procedures
5. **Staged Rollouts**: Regional or percentage-based rollouts for risk mitigation

## Service Dependencies

When planning upgrades, consider the service dependency order:

```
Database Schema
    ↓
Operator Service (control plane)
    ↓
Node Agent (worker nodes)
    ↓
Load Balancer & Edge Proxy (traffic routing)
```

## General Upgrade Process

### 1. Planning Phase

- Review release notes and breaking changes
- Test upgrade in staging environment
- Plan maintenance windows (if needed)
- Prepare rollback procedures
- Notify stakeholders

### 2. Pre-Upgrade Phase

- Backup critical data (automatic with PlanetScale)
- Document current versions
- Baseline performance metrics
- Verify system health
- Stage new binaries

### 3. Execution Phase

- Run database migrations (if required)
- Upgrade services in dependency order
- Monitor health continuously
- Validate functionality after each service
- Document any issues

### 4. Verification Phase

- Run comprehensive health checks
- Verify API functionality
- Check performance metrics
- Validate customer applications
- Monitor for 24 hours

### 5. Cleanup Phase

- Remove old binaries and backups
- Update documentation
- Conduct post-mortem (if issues occurred)
- Plan improvements for next upgrade

## Version Compatibility

### Current Compatibility Matrix

| Component       | Current Version | Compatible With            | Notes                      |
| --------------- | --------------- | -------------------------- | -------------------------- |
| Operator        | v1.2.0          | Node Agent v1.1.x - v1.2.x | API v1                     |
| Node Agent      | v1.2.0          | Operator v1.1.x - v1.2.x   | Firecracker v1.4.x         |
| Load Balancer   | v1.2.0          | All versions               | Stateless                  |
| Edge Proxy      | v1.2.0          | All versions               | Stateless                  |
| Database Schema | v2              | Operator v1.2.x+           | Migration required from v1 |

### Breaking Changes Policy

- Major versions (v1.0.0 → v2.0.0): May include breaking changes
- Minor versions (v1.1.0 → v1.2.0): Backward compatible features
- Patch versions (v1.1.0 → v1.1.1): Bug fixes only

## Upgrade Strategies

### Rolling Update (Default)

Best for: Regular updates, minor versions

```bash
for service in operator node-agent load-balancer edge-proxy; do
    ./upgrade-${service}.sh --rolling
done
```

### Blue-Green Deployment

Best for: Major versions, critical changes

```bash
# Deploy new version alongside old
./deploy-green-cluster.sh

# Switch traffic
./switch-to-green.sh

# Remove blue cluster
./cleanup-blue-cluster.sh
```

### Canary Deployment

Best for: Testing with real traffic

```bash
# Deploy to 10% of nodes
./canary-deploy.sh --percentage 10

# Monitor and increase
./canary-deploy.sh --percentage 50
./canary-deploy.sh --percentage 100
```

## Monitoring During Upgrades

### Key Metrics to Watch

| Metric             | Alert Threshold | Action            |
| ------------------ | --------------- | ----------------- |
| Service Health     | Any unhealthy   | Pause upgrade     |
| Error Rate         | > 1%            | Investigate       |
| Response Time      | > 2x baseline   | Check performance |
| CPU Usage          | > 90%           | Scale or wait     |
| Memory Usage       | > 90%           | Check for leaks   |
| Active Connections | Drop > 10%      | Check routing     |

### Dashboard URLs

- Grafana: `https://metrics.zeitwork.internal`
- Prometheus: `https://prometheus.zeitwork.internal`
- Application Logs: `https://logs.zeitwork.internal`

## Emergency Contacts

During upgrade procedures, ensure access to:

- **On-Call Engineer**: Check PagerDuty schedule
- **Platform Team Lead**: See internal directory
- **Database Admin**: PlanetScale support
- **Customer Success**: For customer communications

## Automation Scripts

All upgrade procedures include automation scripts in `/scripts/upgrade/`:

```bash
scripts/upgrade/
├── upgrade-all.sh           # Full platform upgrade
├── upgrade-operator.sh      # Operator service upgrade
├── upgrade-node-agent.sh    # Node agent upgrade
├── upgrade-load-balancer.sh # Load balancer upgrade
├── upgrade-edge-proxy.sh    # Edge proxy upgrade
├── rollback-all.sh         # Emergency rollback
└── verify-upgrade.sh        # Post-upgrade verification
```

## Best Practices

1. **Never skip versions**: Upgrade sequentially through versions
2. **Test in staging**: Always test the exact upgrade procedure
3. **Monitor actively**: Watch metrics during and after upgrade
4. **Document everything**: Record issues and resolutions
5. **Communicate clearly**: Keep team and customers informed
6. **Plan for rollback**: Always have a way back
7. **Upgrade regularly**: Smaller, frequent updates are safer

## Common Issues and Solutions

### Issue: Service won't start after upgrade

```bash
# Check logs
sudo journalctl -u zeitwork-* -n 100

# Verify permissions
ls -la /usr/local/bin/zeitwork-*

# Check configuration
sudo cat /etc/zeitwork/*.env
```

### Issue: Database connection errors

```bash
# Verify connection string
echo $DATABASE_URL

# Test connectivity
psql $DATABASE_URL -c "SELECT 1;"

# Check PlanetScale status
curl https://status.planetscale.com
```

### Issue: Health checks failing

```bash
# Detailed health check
curl -v http://localhost:8080/health

# Check all services
for port in 8080 8081 8082 8083; do
    curl http://localhost:$port/health
done
```

## Support

For upgrade assistance:

1. Check service-specific upgrade guides in this directory
2. Review troubleshooting sections
3. Contact platform team in #platform-upgrades Slack channel
4. Escalate to on-call engineer if production impact

## Version History

| Date       | Version | Type  | Notes                      |
| ---------- | ------- | ----- | -------------------------- |
| 2024-01-15 | v1.2.0  | Minor | Added multi-region support |
| 2023-12-01 | v1.1.0  | Minor | Performance improvements   |
| 2023-10-15 | v1.0.0  | Major | Initial production release |

---

For specific service upgrade procedures, see the individual guides linked above.
