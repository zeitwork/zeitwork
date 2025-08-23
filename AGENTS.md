# Zeitwork Codebase Guidelines for AI Agents

## Build/Lint/Test Commands

- `make build` - Build all services for Linux AMD64
- `make build-local` - Build for current OS/arch
- `make test` - Run all tests (`go test -v ./...`)
- `make fmt` - Format code (`go fmt ./...`)
- `make lint` - Lint code (`golangci-lint run`)
- `make sqlc` - Generate SQL code from queries

## Code Style

- **Imports**: Grouped stdlib, external deps, internal packages
- **Error handling**: Use custom errors package (`internal/shared/errors`)
- **Database**: Use sqlc for generated code, PostgreSQL with pgx/v5
- **Logging**: Use slog.Logger with structured logging

## TypeScript (packages/database)

- Strict mode enabled, use Drizzle ORM
- Scripts: `bun run db:migrate`, `bun run db:generate`
