# Cubis Workers — Examples

Real-world serverless function examples for all supported runtimes.

## Structure

```
examples/
├── javascript/
│   ├── 01-hello-world.json         → Basic request/response
│   ├── 02-json-api.json            → REST API with routing
│   ├── 03-fetch-proxy.json         → External API proxy
│   ├── 04-auth-webhook.json        → Webhook with signature verification
│   ├── 05-form-handler.json        → Form data processing
│   ├── 06-cron-task.json           → Scheduled task / batch job
│   ├── 07-image-placeholder.json   → Dynamic SVG generation
│   ├── 08-rate-limiter.json        → In-memory rate limiting
│   ├── 09-cors-proxy.json          → CORS proxy for frontend
│   └── 10-crypto-utils.json        → Hashing, encoding, tokens
├── typescript/
│   ├── 01-typed-api.json           → Typed request/response
│   ├── 02-data-transform.json      → ETL / data pipeline
│   └── 03-email-validator.json     → Input validation
├── go/
│   ├── 01-hello-world.json         → Basic Go worker
│   ├── 02-json-processor.json      → JSON transformation
│   ├── 03-fetch-aggregator.json    → Multi-API aggregation
│   ├── 04-hash-service.json        → Crypto utility service
│   └── 05-env-config.json          → Environment-based config
└── README.md
```

## How to Deploy

Each `.json` file is a `POST /api/v1/workers` request body. Deploy with:

```bash
TOKEN="your-jwt-token"
curl -X POST http://localhost:8080/api/v1/workers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @examples/javascript/01-hello-world.json

curl -X POST http://localhost:8080/api/v1/workers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @examples/javascript/02-json-api.json
```

Then invoke:

```bash
curl http://localhost:8080/api/v1/invoke/hello-world

curl -X POST http://localhost:8080/api/v1/invoke/json-api \
  -H "Content-Type: application/json" \
  -d '{"action":"list"}'
```
