# Cubis Workers (wkr)

A high-performance, security-first serverless platform supporting **Go** and **JavaScript/TypeScript** runtimes.

## Architecture

```
cmd/server/          → Entry point
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

### Workflow

```bash
# 1. Login to your Cubis Workers instance
wkr-cli login --api-url http://localhost:8080 --email dev@example.com

# 2. Initialize a new worker project
mkdir my-worker && cd my-worker
wkr-cli init --name my-worker --runtime javascript

# 3. Edit your worker code (worker.js created automatically)
# 4. Deploy
wkr-cli deploy

# 5. Invoke
wkr-cli invoke my-worker

# 6. List all workers
wkr-cli list

# 7. Delete
wkr-cli delete my-worker
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

## API Endpoints

### Auth

| Method | Path                    | Description             |
| ------ | ----------------------- | ----------------------- |
| POST   | `/api/v1/auth/register` | Register a new user     |
| POST   | `/api/v1/auth/login`    | Login and get JWT token |

### Workers (requires JWT)

| Method | Path                  | Description        |
| ------ | --------------------- | ------------------ |
| POST   | `/api/v1/workers`     | Create a worker    |
| GET    | `/api/v1/workers`     | List your workers  |
| GET    | `/api/v1/workers/:id` | Get worker details |
| PUT    | `/api/v1/workers/:id` | Update a worker    |
| DELETE | `/api/v1/workers/:id` | Delete a worker    |

### Invoke (public, rate-limited)

| Method   | Path                   | Description              |
| -------- | ---------------------- | ------------------------ |
| POST/GET | `/api/v1/invoke/:name` | Execute a worker by name |

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
  -d '{"email":"dev@example.com","name":"Dev","password":"123"}'

# Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@example.com","password":"123"}' | jq -r '.data.token')

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

# Invoke the worker
curl http://localhost:8080/api/v1/invoke/hello-world
```

## Security Features

- JWT authentication with HMAC-SHA256 (no external JWT library)
- bcrypt password hashing
- Rate limiting on public endpoints
- Security headers (HSTS, CSP, X-Frame-Options, etc.)
- Sandboxed worker execution with timeout enforcement
- Concurrency-limited execution pool
- SQL injection prevention via GORM parameterized queries
- Input validation on all endpoints
- Graceful shutdown with resource cleanup

## Configuration

All settings in `config.yml` with environment variable overrides via Viper.

## License

MIT
