# Cubis Workers (wkr)

A high-performance, security-first serverless platform supporting **Go** and **JavaScript/TypeScript** runtimes.

## Architecture

```
cmd/
  server/            → Server entry point
  cli/               → CLI tool (wkr-cli)
internal/
  config/            → Viper-based YAML configuration
  database/          → PostgreSQL connection (GORM)
  model/             → Domain models (User, Worker, Deployment, Invocation)
  repository/        → Data access layer
  service/           → Business logic (Auth, Worker management)
  handler/           → HTTP handlers (Fiber v3)
  middleware/         → Auth (JWT), rate limiting, security headers
  runtime/           → Sandboxed worker execution engine
  server/            → Fiber app setup and routing
```

## Tech Stack

- **Go 1.25** — core platform language
- **GoFiber v3** — high-performance HTTP framework
- **PostgreSQL + GORM** — persistent storage with ORM
- **Viper** — YAML configuration management
- **JWT (HMAC-SHA256)** — zero-dependency authentication
- **Deno/Node.js** — JavaScript/TypeScript runtime execution

## Quick Start

```bash
# Start PostgreSQL
docker compose up db -d

# Run the server
make dev

# Or build and run
make build && make run
```

## CLI (wkr-cli)

Deploy functions from your terminal — like Wrangler, but for Cubis Workers.

```bash
# Build the CLI
make build-cli

# Or install globally
make install-cli
```

### Commands

| Command            | Description                              |
| ------------------ | ---------------------------------------- |
| `login`            | Authenticate with the Cubis Workers API  |
| `logout`           | Remove stored credentials                |
| `whoami`           | Show current authenticated user          |
| `init`             | Initialize a new worker project          |
| `deploy`           | Deploy the current worker to the platform|
| `list`, `ls`       | List your deployed workers               |
| `invoke`           | Invoke a worker by name                  |
| `delete`, `rm`     | Delete a worker by name                  |
| `revisions`, `rev` | List deployment revisions for a worker   |
| `rollback`         | Rollback a worker to a specific version  |

### Workflow

```bash
# 1. Login
wkr-cli login --api-url http://localhost:8080 --email dev@example.com

# 2. Check who you're logged in as
wkr-cli whoami

# 3. Initialize a new worker project (creates subfolder)
wkr-cli init --name my-worker --template json-api

# 4. Or init in current directory
wkr-cli init --runtime javascript

# 5. Edit your worker code, then deploy
cd my-worker
wkr-cli deploy

# 6. Invoke
wkr-cli invoke my-worker

# 7. View revision history
wkr-cli revisions my-worker

# 8. Rollback to a previous version
wkr-cli rollback my-worker --version 1

# 9. List all workers
wkr-cli list

# 10. Delete
wkr-cli delete my-worker

# 11. Logout
wkr-cli logout
```

### Init Options

```bash
wkr-cli init [options]

--name <name>        Worker name (creates subfolder if set, otherwise uses current dir)
--runtime <rt>       Runtime: go, javascript, typescript (default: javascript)
--template <tpl>     Use a prebuilt template
--list-templates     List available templates
```

### Templates

| Template   | Runtime    | Description                |
| ---------- | ---------- | -------------------------- |
| `hello-js` | javascript | Basic hello world          |
| `hello-ts` | typescript | Basic hello world (typed)  |
| `hello-go` | go         | Basic hello world          |
| `json-api` | javascript | JSON request/response API  |
| `cron`     | javascript | Cron-style scheduled task  |
| `proxy`    | javascript | Request proxy/forwarder    |

```bash
# List all templates
wkr-cli init --list-templates

# Init with a template
wkr-cli init --name my-api --template json-api
```

### Project Config (wkr.yaml)

```yaml
name: my-worker
runtime: javascript
entry_point: main
main: worker.js
env_vars:
  API_KEY: secret123
```

### Revisions & Rollback

Every deploy creates a revision snapshot. You can view history and rollback to any version:

```bash
wkr-cli revisions my-worker
# VERSION  STATUS       ENTRY        HASH       DEPLOYED AT
# v3       active       main         a1b2c3d4   2026-02-15 12:50:00
# v2       active       main         e5f6a7b8   2026-02-15 12:45:00
# v1       active       main         c9d0e1f2   2026-02-15 12:40:00

wkr-cli rollback my-worker --version 1
# ✓ Rolled back my-worker to v1 (now at v4, active)
```

## API Endpoints

### Auth

| Method | Path                    | Description             |
| ------ | ----------------------- | ----------------------- |
| POST   | `/api/v1/auth/register` | Register a new user     |
| POST   | `/api/v1/auth/login`    | Login and get JWT token |

### Workers (requires JWT)

| Method | Path                                    | Description                  |
| ------ | --------------------------------------- | ---------------------------- |
| POST   | `/api/v1/workers`                       | Create a worker              |
| GET    | `/api/v1/workers`                       | List your workers            |
| GET    | `/api/v1/workers/:id`                   | Get worker details           |
| PUT    | `/api/v1/workers/:id`                   | Update a worker              |
| DELETE | `/api/v1/workers/:id`                   | Delete a worker              |
| PUT    | `/api/v1/workers/by-name/:name`         | Update a worker by name      |
| DELETE | `/api/v1/workers/by-name/:name`         | Delete a worker by name      |
| GET    | `/api/v1/workers/by-name/:name/revisions` | List deployment revisions  |
| POST   | `/api/v1/workers/by-name/:name/rollback`  | Rollback to a version      |

### Invoke (public, rate-limited)

| Method   | Path                              | Description                        |
| -------- | --------------------------------- | ---------------------------------- |
| POST/GET | `/api/v1/invoke/@:username/:name` | Execute a worker (scoped by user)  |
| POST/GET | `/api/v1/invoke/:name`            | Execute a worker (legacy, global)  |

Worker names are scoped per-user. Two different users can have workers with the same name. The scoped invoke URL uses the `@username` prefix:

```
https://your-instance.com/api/v1/invoke/@dev/hello-world
```

### Health

| Method | Path      | Description     |
| ------ | --------- | --------------- |
| GET    | `/health` | Health check    |
| GET    | `/ready`  | Readiness probe |

## Usage Example

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@example.com","name":"Dev","password":"securepass"}'

# Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@example.com","password":"securepass"}' | jq -r '.data.token')

# Create a JavaScript worker
curl -X POST http://localhost:8080/api/v1/workers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "hello-world",
    "runtime": "javascript",
    "code": "function main(req) { return { message: \"Hello from Cubis Workers!\" }; }",
    "entry_point": "main"
  }'

# Invoke the worker (scoped)
curl http://localhost:8080/api/v1/invoke/@dev/hello-world

# Invoke the worker (legacy)
curl http://localhost:8080/api/v1/invoke/hello-world
```

## Security Features

- JWT authentication with HMAC-SHA256 (no external JWT library)
- Per-user scoped worker names (no global name squatting)
- bcrypt password hashing
- Rate limiting on public endpoints
- Security headers (HSTS, CSP, X-Frame-Options, etc.)
- Sandboxed worker execution with timeout enforcement
- Concurrency-limited execution pool
- SQL injection prevention via GORM parameterized queries
- Input validation on all endpoints
- Encrypted environment variables at rest
- Graceful shutdown with resource cleanup

## Configuration

All settings in `config.yml` with environment variable overrides via Viper.

## Build

```bash
make build          # Build server
make build-cli      # Build CLI
make build-all      # Build both
make install-cli    # Install CLI to PATH
make dev            # Run server in dev mode
make test           # Run tests
```

## License

MIT
