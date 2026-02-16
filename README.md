# Cubis Workers (wkr)

A high-performance, security-first serverless platform supporting **Go**, **JavaScript/TypeScript**, and **Python** runtimes with edge node distribution.

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
  service/           → Business logic (Auth, Worker management, LogBus)
  handler/           → HTTP handlers (Fiber v3) + WebSocket log streaming
  middleware/         → Auth (JWT), rate limiting, security headers
  runtime/           → Sandboxed worker execution engine
  server/            → Fiber app setup and routing
  edge/              → Edge node registry, routing, and failover
```

## Tech Stack

- **Go 1.25** — core platform language
- **GoFiber v3** — high-performance HTTP framework
- **PostgreSQL + GORM** — persistent storage with ORM
- **Viper** — YAML configuration management
- **JWT (HMAC-SHA256)** — zero-dependency authentication
- **Deno/Node.js** — JavaScript/TypeScript runtime execution
- **Python 3** — Python runtime execution
- **WebSocket** — real-time log streaming (gofiber/contrib)

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

### Install

One-line install (Linux, macOS, Windows/Git Bash):

```bash
curl -fsSL https://raw.githubusercontent.com/cubetiqlabs/wkr/main/scripts/install.sh | sh
```

Or build from source:

```bash
make build-cli      # builds ./bin/wkr
make install-cli    # installs to PATH
```

### Update

```bash
wkr update
```

### Commands

| Command            | Description                              |
| ------------------ | ---------------------------------------- |
| `login`            | Authenticate with the Cubis Workers API  |
| `logout`           | Remove stored credentials                |
| `whoami`           | Show current authenticated user          |
| `init`             | Initialize a new worker project          |
| `deploy`           | Deploy the current worker to the platform|
| `dev`              | Run worker locally (no deploy needed)    |
| `list`, `ls`       | List your deployed workers               |
| `invoke`           | Invoke a worker by name                  |
| `delete`, `rm`     | Delete a worker by name                  |
| `revisions`, `rev` | List deployment revisions for a worker   |
| `rollback`         | Rollback a worker to a specific version  |
| `logs`             | View invocation logs for a worker        |
| `update`, `upgrade`| Update wkr to the latest version         |

### Workflow

```bash
# 1. Login
wkr-cli login --api-url http://localhost:8080 --email dev@example.com

# 2. Check who you're logged in as
wkr-cli whoami

# 3. Initialize a new worker project (creates subfolder)
wkr-cli init --name my-worker --template json-api

# 4. Or init in current directory
wkr-cli init --runtime python

# 5. Edit your worker code, then deploy
cd my-worker
wkr-cli deploy

# 6. Run locally without deploying
wkr-cli dev --body '{"name":"test"}' --query 'page=1'

# 7. Invoke remotely
wkr-cli invoke my-worker

# 8. View logs (recent or real-time)
wkr-cli logs my-worker
wkr-cli logs my-worker -f
wkr-cli logs my-worker -f -v

# 9. View revision history
wkr-cli revisions my-worker

# 10. Rollback to a previous version
wkr-cli rollback my-worker --version 1

# 11. List all workers
wkr-cli list

# 12. Delete
wkr-cli delete my-worker

# 13. Logout
wkr-cli logout
```

### Init Options

```bash
wkr-cli init [options]

--name <name>        Worker name (creates subfolder if set, otherwise uses current dir)
--runtime <rt>       Runtime: go, javascript, typescript, python (default: javascript)
--runtime-version <v> Runtime version: e.g. 1.24, 3.12, 22
--template <tpl>     Use a prebuilt template
--list-templates     List available templates
```

### Templates

| Template   | Runtime    | Description                |
| ---------- | ---------- | -------------------------- |
| `hello-js` | javascript | Basic hello world          |
| `hello-ts` | typescript | Basic hello world (typed)  |
| `hello-go` | go         | Basic hello world          |
| `hello-py` | python     | Basic hello world          |
| `json-api` | javascript | JSON request/response API  |
| `cron`     | javascript | Cron-style scheduled task  |
| `proxy`    | javascript | Request proxy/forwarder    |

```bash
# List all templates
wkr-cli init --list-templates

# Init with a template
wkr-cli init --name my-api --template json-api
wkr-cli init --name my-py --template hello-py
```

### Local Development

Run workers locally without deploying:

```bash
wkr-cli dev                                          # GET /
wkr-cli dev --method POST --body '{"name":"test"}'   # POST with body
wkr-cli dev --query 'page=2&sort=name'               # with query params
wkr-cli dev --path '/v1/users'                       # with sub-path
```

### Logs

View invocation logs (recent history or real-time streaming):

```bash
wkr-cli logs my-worker              # last 20 logs
wkr-cli logs my-worker --limit 50   # last 50 logs
wkr-cli logs my-worker -f           # follow in real-time (WebSocket)
wkr-cli logs my-worker -f -v        # follow with verbose details
```

Output:
```
15:30:05 ✓ 200 GET / 12ms [a1b2c3d4]
15:30:12 ✗ 500 POST /v1/users 3ms [e5f6a7b8]
  error: NameError: name 'x' is not defined
  log: processing request...
  --- stack trace ---
  Traceback (most recent call last):
    File "worker.py", line 5, in main
  NameError: name 'x' is not defined
  ---
```

With `-v` (verbose):
```
15:30:05 ✓ 200 GET / 12ms [a1b2c3d4]
  node=edge-us-1 region=us-east-1 ip=203.0.113.42 in=45B out=128B
  ua=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)
```

The `-f` flag uses WebSocket with auto-reconnect — survives network blips and server restarts.

### Project Config (wkr.yaml)

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/cubetiqlabs/wkr/main/schemas/wkr.schema.json
name: my-worker
runtime: python
runtime_version: "3.12"       # optional: specific runtime version (e.g. 1.24, 3.12, 22)
entry_point: main
main: worker.py
package_manager: pip          # optional: npm, yarn, pnpm, bun, deno, pip, uv, go (auto-detected)
env_vars:
  API_KEY: secret123
```

A JSON Schema is provided at `schemas/wkr.schema.json` for auto-completion and validation in any editor. Works automatically in VS Code (with Red Hat YAML extension), JetBrains IDEs, Neovim (yaml-language-server), and any editor supporting the `# yaml-language-server` directive.

#### Schema Setup

**VS Code** — works out of the box via `.vscode/settings.json` (included in repo). Just install the [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml).

**JetBrains (IntelliJ, WebStorm, GoLand)** — auto-detects from the `# yaml-language-server` comment in generated `wkr.yaml` files, or configure manually: Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings → add `schemas/wkr.schema.json` for `wkr.yaml`.

**Neovim / Any LSP editor** — the `# yaml-language-server` comment at the top of `wkr.yaml` activates the schema automatically when using yaml-language-server.

**New projects** — `wkr init` automatically adds the schema comment to generated `wkr.yaml` files. For existing projects, add this as the first line:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/cubetiqlabs/wkr/main/schemas/wkr.schema.json
```

### Runtime Versions

Pin a specific runtime version in `wkr.yaml`. The platform looks for a versioned binary on PATH and falls back to the system default.

| Runtime    | `runtime_version` | Binary looked up     | Install method                                      |
| ---------- | ----------------- | -------------------- | --------------------------------------------------- |
| go         | `1.24`            | `go1.24`             | `go install golang.org/dl/go1.24@latest && go1.24 download` |
| python     | `3.12`            | `python3.12`         | pyenv, system package manager, or deadsnakes PPA    |
| javascript | `22`              | `node22`             | nvm, fnm, or symlink                                |
| typescript | `22`              | `node22`             | nvm, fnm, or symlink                                |

### Dependencies

Dependencies are auto-detected and installed before both `wkr dev` and `wkr deploy`:

| Runtime    | Detected file        | Default PM | Supported PMs              |
| ---------- | -------------------- | ---------- | -------------------------- |
| javascript | `package.json`       | npm        | npm, yarn, pnpm, bun, deno |
| typescript | `package.json`       | npm        | npm, yarn, pnpm, bun, deno |
| python     | `requirements.txt`   | pip        | pip, uv                    |
| go         | `go.mod`             | go         | go                         |

The package manager is auto-detected from lockfiles:

| Lockfile            | Detected PM |
| ------------------- | ----------- |
| `deno.lock`         | deno        |
| `deno.json`         | deno        |
| `pnpm-lock.yaml`    | pnpm        |
| `yarn.lock`         | yarn        |
| `bun.lockb`         | bun         |
| `package-lock.json` | npm         |

Override with `package_manager` in `wkr.yaml`:

```yaml
name: my-api
runtime: javascript
main: worker.js
package_manager: pnpm
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

## Nested Routes

Workers support sub-paths. The first path segment is the worker name, the rest is passed as `req.path`:

```
GET /api/v1/invoke/my-api/v1/users?page=2
→ worker: my-api, path: /v1/users, query: {page: "2"}

POST /api/v1/invoke/@dev/my-api/v1/users/123
→ worker: my-api, path: /v1/users/123 (scoped to @dev)
```

## API Endpoints

### Auth

| Method | Path                    | Description             |
| ------ | ----------------------- | ----------------------- |
| POST   | `/api/v1/auth/register` | Register a new user     |
| POST   | `/api/v1/auth/login`    | Login and get JWT token |

### Workers (requires JWT)

| Method | Path                                      | Description                  |
| ------ | ----------------------------------------- | ---------------------------- |
| POST   | `/api/v1/workers`                         | Create a worker              |
| GET    | `/api/v1/workers`                         | List your workers            |
| GET    | `/api/v1/workers/:id`                     | Get worker details           |
| PUT    | `/api/v1/workers/:id`                     | Update a worker              |
| DELETE | `/api/v1/workers/:id`                     | Delete a worker              |
| PUT    | `/api/v1/workers/by-name/:name`           | Update a worker by name      |
| DELETE | `/api/v1/workers/by-name/:name`           | Delete a worker by name      |
| GET    | `/api/v1/workers/by-name/:name/revisions` | List deployment revisions    |
| POST   | `/api/v1/workers/by-name/:name/rollback`  | Rollback to a version        |
| GET    | `/api/v1/workers/by-name/:name/logs`      | List invocation logs         |

### Log Streaming (WebSocket)

| Path                            | Description                          |
| ------------------------------- | ------------------------------------ |
| `/api/v1/ws/logs/:name?token=T` | Real-time log stream via WebSocket  |

### Invoke (public, rate-limited, supports nested routes)

| Method   | Path                                | Description                        |
| -------- | ----------------------------------- | ---------------------------------- |
| POST/GET | `/api/v1/invoke/@:username/:name/*` | Execute a worker (scoped by user)  |
| POST/GET | `/api/v1/invoke/:name/*`            | Execute a worker (legacy, global)  |

Worker names are scoped per-user. Two different users can have workers with the same name. The scoped invoke URL uses the `@username` prefix:

```
https://your-instance.com/api/v1/invoke/@dev/hello-world
https://your-instance.com/api/v1/invoke/@dev/my-api/v1/users?page=2
```

### Health

| Method | Path      | Description     |
| ------ | --------- | --------------- |
| GET    | `/health` | Health check    |
| GET    | `/ready`  | Readiness probe |

## Runtimes

### JavaScript / TypeScript

```javascript
function main(req) {
  const name = req.query.name || "world";
  return { message: `Hello, ${name}!` };
}
```

Built-in helpers: `env(key, default)`, `log(...)`, `fetch()`, `btoa()`, `atob()`, `sha256()`, `md5()`, `uuid()`, `sleep(ms)`, `jsonParse()`, `jsonStringify()`.

### Go

```go
package main

func main(req map[string]interface{}) map[string]interface{} {
    return map[string]interface{}{
        "message": "Hello from Go!",
    }
}
```

Built-in helpers: `Log(...)`, `Env(key)`.

### Python

```python
def main(req):
    name = req.get("query", {}).get("name", "world")
    return {"message": f"Hello, {name}!"}
```

Built-in helpers: `env(key, default)`, `log(...)`.

All runtimes receive a request object with: `method`, `path`, `query` (object), `headers` (object), `body` (string).

## Invocation Logs

Every invocation is recorded with:

- Request ID, status code, duration
- HTTP method, path, query string
- Client IP, user-agent
- Node ID, region (which edge node executed it)
- Request/response bytes
- Worker stderr logs (console.log / log() output)
- Error message and stack trace (on crash/panic)

Logs from edge-executed workers are pushed back to the origin node for unified streaming.

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

# Create a Python worker
curl -X POST http://localhost:8080/api/v1/workers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "hello-world",
    "runtime": "python",
    "code": "def main(req):\n    return {\"message\": \"Hello from Python!\"}",
    "entry_point": "main"
  }'

# Invoke the worker
curl http://localhost:8080/api/v1/invoke/@dev/hello-world

# Invoke with nested route
curl http://localhost:8080/api/v1/invoke/@dev/hello-world/v1/greet?name=Kiro
```

## Security Features

- JWT authentication with HMAC-SHA256 (no external JWT library)
- Per-user scoped worker names (no global name squatting)
- bcrypt password hashing
- Rate limiting on public endpoints
- Security headers (HSTS, CSP, X-Frame-Options, etc.)
- Sandboxed worker execution with timeout enforcement
- Concurrency-limited execution pool
- Pre-deploy code verification (compile/syntax check)
- SQL injection prevention via GORM parameterized queries
- Input validation on all endpoints
- Encrypted environment variables at rest
- Graceful shutdown with resource cleanup
- Panic-safe invocation logging (defer + recover)

## Edge Cluster

- Multi-node edge distribution with automatic load balancing
- Heartbeat-based health monitoring
- Failover routing when nodes go down
- Log forwarding from edge nodes back to origin for unified streaming
- Internal secret-authenticated inter-node communication

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
