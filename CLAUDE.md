# Courier

Messaging gateway service for RapidPro/TextIt. Handles incoming and outgoing messages across 70+ channel integrations (SMS, WhatsApp, Telegram, etc.).

## Language & Stack

- **Go 1.25** with Chi HTTP router
- PostgreSQL 15, Valkey (Redis), AWS (S3, DynamoDB, CloudWatch)
- Docker/devcontainer for local development

## Build & Test

```bash
# run all tests (sequential to avoid race conditions)
go test -p=1 ./...

# run tests with coverage
go test -p=1 -coverprofile=coverage.text -covermode=atomic ./...

# run tests for a specific handler
go test -p=1 ./handlers/telegram/...

# build binary
go build ./cmd/courier
```

Tests require PostgreSQL, Valkey, and LocalStack services (provided by the devcontainer network or CI).

## Project Structure

```
cmd/courier/          # main entry point
handlers/             # 70+ channel handler implementations (one subdir per channel type)
backends/rapidpro/    # RapidPro database backend (ongoing process of moving stuff into models)
core/models/          # domain models (Channel, Msg, Contact, URN, etc.)
runtime/              # configuration and runtime setup
test/                 # mock backend, test utilities
testsuite/            # test database and runtime setup
utils/                # utilities (clogs, queue, URL handling)
```

## Key Architecture

- **Interface-driven**: core interfaces (`Channel`, `Backend`, `ChannelHandler`, `Msg`, `Event`) defined in root package
- **Handler registration**: each handler calls `courier.RegisterHandler()` in its `init()` function
- **Routes**: channel callbacks mounted at `/c/{channel-type}/{uuid}/...`
- **Channel types**: identified by 2-3 letter codes (e.g., "EX" for External, "TG" for Telegram)

## Adding/Modifying Handlers

Each handler lives in `handlers/{name}/` with:
- `handler.go` - implementation of `ChannelHandler` interface
- `handler_test.go` - tests using `IncomingTestCase` / `OutgoingTestCase` struct patterns

Handlers extend `handlers.BaseHandler` and register routes via `AddHandlerRoute()`.

## Conventions

- Line length: 120 characters
- Format with `gofmt` (editor formatOnSave enabled)
- Tests use `github.com/stretchr/testify` (assert/require)
- Tests run sequentially (`-p=1`) to avoid database conflicts
- Strong UUID typing: `ChannelUUID`, `MsgUUID`, `ContactUUID`, etc.
- Config keys use constants: `ConfigAPIKey`, `ConfigAuthToken`, etc.

## CI

GitHub Actions runs tests on push/PR using `golang:1.25-trixie` with PostgreSQL 15, Valkey 8.0, and LocalStack. Coverage uploaded to Codecov.
