# Zeitwork CLI

The Zeitwork CLI is a command-line tool for deploying and managing the Zeitwork platform.

## Configuration

The CLI supports loading configuration from a `.env` file in the project root. This allows you to pre-configure deployment settings.

### Environment Variables

Create a `.env` file in the project root with the following variables:

```bash
# Database Configuration (required)
DATABASE_URL=postgresql://user:password@host.connect.psdb.cloud/zeitwork-production?sslmode=require

# Operator Nodes (required - comma-separated list of IP addresses)
# At least 3 operators are required for production
OPERATORS=10.0.1.10,10.0.1.11,10.0.1.12

# Worker Nodes (required - comma-separated list of IP addresses)
# At least 3 workers are required for production
WORKERS=10.0.2.10,10.0.2.11,10.0.2.12,10.0.2.13,10.0.2.14

# Region Configuration (optional - will prompt if not provided)
REGION_NAME=US East
REGION_CODE=us-east-1
REGION_COUNTRY=US

# SSH Key Path (optional - will prompt to generate if not provided)
SSH_KEY_PATH=~/.ssh/id_rsa
```

## Features

### Automatic Configuration Loading

When you run the CLI, it will automatically:

1. Check for a `.env` file in the current directory
2. Load `DATABASE_URL`, `OPERATORS`, and `WORKERS` if present
3. Parse comma-separated IP addresses from `OPERATORS` and `WORKERS`
4. Skip configuration steps that have been pre-filled

### Database Reset

If the database already contains Zeitwork tables, the CLI will:

1. Detect existing tables during the database check
2. Ask if you want to reset the database
3. Drop all existing tables if you confirm (WARNING: This deletes all data!)
4. Continue with fresh deployment

### Interactive Setup

If configuration is missing from the `.env` file, the CLI will prompt for:

- Database connection URL (if not in .env)
- Region configuration (name, code, country)
- SSH key generation or path
- Operator node IP addresses (if not in .env)
- Worker node IP addresses (if not in .env)

## Usage

```bash
# Run the CLI
go run cmd/zeitwork-cli/main.go

# Or if built
./zeitwork-cli
```

## Deployment Process

The CLI performs the following steps:

1. **Configuration**: Load from .env or prompt interactively
2. **Database Check**: Verify connection and check for existing tables
3. **Database Setup**: Run migrations and initialize regions
4. **Build Binaries**: Compile all Zeitwork services
5. **Deploy Operators**: Install and configure operator nodes
6. **Deploy Workers**: Install and configure worker nodes
7. **Verification**: Check all services are healthy

## Files Created

The CLI creates the following files in `.deploy/`:

- `config.json` - Deployment configuration (non-sensitive)
- `inventory.json` - Node inventory and SSH key path
- `database.env` - Database connection string
- `id_rsa` / `id_rsa.pub` - SSH keys (if generated)
- `zeitwork-binaries.tar.gz` - Compiled binaries package
