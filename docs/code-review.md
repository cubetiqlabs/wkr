# Code Review: `wkr` — Serverless Worker Runtime

Date: 10 Feb 2026

## Summary

Cubis `wkr` is a well-architected Go serverless worker runtime supporting Go, JavaScript, and TypeScript execution with edge computing, JWT authentication, per-user quotas, encrypted env vars, and full Prometheus observability. The codebase follows clean layered architecture (handler → service → repository), uses idiomatic Go patterns, and has strong operational tooling. However, there are critical security gaps in the sandbox execution model, missing input validation, zero test files, and several logic bugs that need attention before production use.

## **Verdict**: [x] Request Changes

## Critical Issues (Must Fix)

### 1. sandbox.go Security: Sandbox Escape via Bypass of String-Based Validation

The security `Validator` uses `strings.Contains` to block dangerous imports/globals. This is trivially bypassed:

- **Go**: Aliased imports (`import x "os/exec"`), `go:linkname`, `reflect`, build tags, or string concatenation to reconstruct blocked tokens at runtime.
- **JS/TS**: Deno `eval` mode with `--no-remote` alone still allows local filesystem reads, writes, FFI, and subprocess spawning.

```go
// Current (bypassable) — validator.go
if strings.Contains(code, `"`+imp+`"`) {
    return fmt.Errorf("blocked import: %s", imp)
}

// Suggested — use Go AST parser for Go code:
fset := token.NewFileSet()
f, err := parser.ParseFile(fset, "", wrappedCode, parser.ImportsOnly)
for _, imp := range f.Imports {
    path := strings.Trim(imp.Path.Value, `"`)
    if slices.Contains(blocked, path) { return err }
}

// For Deno, add all deny flags:
args = []string{"eval", "--no-remote", "--no-read", "--no-write",
    "--no-run", "--no-ffi", "--no-env", code}
```

- **Impact**: Full remote code execution — an attacker can spawn processes, read the filesystem, or exfiltrate data from the host.

### 2. config.yml Security: Hardcoded Secrets Shipped in Repository

```yaml
jwt_secret: change-me-in-production-use-env-var
encryption_key: change-me-32-char-secret-key!!
password: cubis_secret
```

These defaults are committed to the repository. There is **no startup validation** to reject them in non-development environments.

- **Current**: `config.Load()` silently accepts default secrets.
- **Suggested**: Add a guard in `config.Load()`:

```go
if cfg.App.Env != "development" {
    if cfg.Auth.JWTSecret == "change-me-in-production-use-env-var" {
        return nil, fmt.Errorf("JWT secret must be changed in %s environment", cfg.App.Env)
    }
    if strings.Contains(cfg.Runtime.EncryptionKey, "change-me") {
        return nil, fmt.Errorf("encryption key must be changed in %s environment", cfg.App.Env)
    }
}
```

- **Impact**: Token forgery, env var decryption, full account takeover if defaults are used in production.

### 3. router.go Security: Internal Edge Endpoints Have No Authentication

`SyncWorkerToEdge` calls `/internal/sync/worker` with just an `X-Cubis-Internal: true` header — no shared secret, no mTLS.

```go
// Current
req.Header.Set("X-Cubis-Internal", "true")

// Suggested — add a shared secret:
req.Header.Set("X-Cubis-Internal-Secret", r.internalSecret)
// And verify on the receiving end
```

- **Impact**: Any network-reachable attacker can push arbitrary worker code to edge nodes.

### 4. invoke_handler.go Logic: Quota Check Failures Allow Unlimited Invocations

```go
if err := h.quotaService.CheckInvocationAllowed(c.Context(), worker.OwnerID); err != nil {
    switch err {
    case service.ErrQuotaRequestsExceeded:
        // ... returns error
    default:
        logger.Error("quota check failed", zap.Error(err))
        // FALLS THROUGH — request proceeds!
    }
}
```

When the quota check returns an unexpected error (DB down, connection timeout), the request is **allowed to proceed** instead of being rejected. During a database outage, quotas are effectively disabled.

- **Suggested**: Return a 503 in the `default` case:

```go
default:
    logger.Error("quota check failed", zap.Error(err))
    return errResponse(c, fiber.StatusServiceUnavailable, "service temporarily unavailable")
```

- **Impact**: Unlimited free invocations during any database incident.

### 5. No Test Files Exist

```
$ find . -name "*_test.go"
(empty)
```

The Makefile has `test: go test -race -cover projects.` but there are **zero test files** in the codebase. For a system that executes arbitrary user code, this is a critical gap — security validators, JWT handling, crypto, quota logic, and edge routing are all untested.

- **Impact**: No confidence in correctness; regressions will go undetected.

---

## Major Issues (Should Fix)

### 6. audit.go Logic: Audit Log Uses Cancelled Context in Goroutine

```go
go func() {
    if err := a.db.WithContext(ctx).Create(entry).Error; err != nil {
```

The goroutine captures the HTTP request's `ctx`. When the request completes, the context is cancelled, and the DB write will fail. Audit logs are security-critical.

- **Suggested**: Use `context.WithoutCancel(ctx)` (Go 1.21+) or `context.Background()` with a timeout.

### 7. config.go Logic: Custom `itoa()` Handles Only Positive Integers

```go
func itoa(i int) string {
    if i == 0 { return "0" }
    s := ""
    for i > 0 { s = string(rune('0'+i%10)) + s; i /= 10 }
    return s
}
```

Returns empty string `""` for negative inputs. This is used in `DSN()` for port numbers. The same anti-pattern exists in invoke_handler.go with `itoa64`.

- **Suggested**: Replace both with `strconv.Itoa` / `strconv.FormatInt`. No reason to reimplement stdlib.

### 8. config.go Logic: DSN Password Not Escaped

```go
func (d DatabaseConfig) DSN() string {
    return "host=" + d.Host + " port=" + itoa(d.Port) +
        " user=" + d.User + " password=" + d.Password + ...
```

If the DB password contains spaces or `=` characters, the DSN will be malformed. Use a `url.URL` builder or the `pgx` connection string format with proper quoting.

### 9. router.go Security: CORS Wildcard in Production

```go
AllowOrigins: []string{"*"},
```

Wildcard CORS allows cross-origin requests from any website. This should be configurable and restricted in production.

### 10. sandbox.go Logic: Payload Passed via Environment Variable

```go
env = append(env, "CUBIS_PAYLOAD="+string(payload))
```

OS environment variables have platform-specific size limits (~128KB–2MB). Large request payloads will cause silent exec failures. Pass via stdin or a temp file instead.

### 11. auth_handler.go Logic: No Input Validation Beyond JSON Binding

Structs use `validate` tags (e.g., `validate:"required,email"`) but no validation library is actually invoked. `c.Bind().Body()` only performs JSON deserialization. Email format, password length, and worker name constraints are **never enforced**.

```go
// Current — only binds JSON, tags are decorative
if err := c.Bind().Body(&input); err != nil { ... }

// Suggested — add go-playground/validator
validate := validator.New()
if err := validate.Struct(&input); err != nil { ... }
```

### 12. ratelimit.go Logic: Rate Limiter Goroutine Leak

`NewRateLimiter` spawns a cleanup goroutine with no shutdown mechanism — `for range ticker.C` runs forever.

- **Suggested**: Accept a `context.Context` or add a `Stop()` method that closes a done channel.

---

## Minor Issues (Nice to Have)

### 13. router.go Code Smell: Redeclares Go Builtin `max()`

Go 1.21+ has a builtin `max()`. This custom implementation shadows it. Remove it.

### 14. sandbox.go Hardening: Cache Directory Permissions

```go
os.MkdirAll(cacheDir, 0o755)
```

Compiled user binaries in `/tmp/cubis-cache` are world-readable. Use `0o700`.

### 15. worker_service.go Error Handling: Deployment Creation Silently Discarded

```go
_ = s.deploymentRepo.Create(ctx, dep)
```

Failed deployment records are silently ignored, leading to inconsistent audit trails.

### 16. docker-compose.yml Config: Edge Node Uses Same Default Secrets

Both config.yml and config-edge.yml ship identical default JWT secrets and DB passwords. The docker-compose doesn't override them via environment.

### 17. crypto.go Crypto: Key Derivation Uses Raw SHA-256

```go
func deriveKey(key string) []byte {
    h := sha256.Sum256([]byte(key))
    return h[:]
}
```

A single SHA-256 round is not a proper KDF. For production use, consider `argon2id` or `scrypt` (or at minimum `HKDF`) for key derivation from a passphrase.

### 18. Naming: `itoa64` in invoke_handler.go and `itoa` in config.go

Two separate reimplementations of integer-to-string in different packages. DRY violation — both should just use `strconv`.

---

## Positive Feedback

- **Clean layered architecture**: Handler → Service → Repository separation is consistent and well-applied across the entire codebase. Dependencies flow inward, and each layer has a clear responsibility.
- **Encryption at rest for env vars**: Worker environment variables are encrypted with AES-256-GCM before DB storage and decrypted only at invocation time — excellent security practice.
- **Env var masking in API responses**: `maskEnvVars()` replaces all values with `***` for GET responses. Smart defense-in-depth.
- **Go binary compilation cache**: The `sync.Map`-based binary cache with SHA-256 keying avoids redundant compilation — good performance optimization with the pool/buffer reuse pattern.
- **Comprehensive Prometheus instrumentation**: 20+ metrics covering invocations, pool utilization, edge cluster health, auth, quotas, and bandwidth. Route patterns used instead of actual paths to avoid cardinality explosion.
- **Graceful shutdown**: Proper `signal.NotifyContext` → registry deregister → pool shutdown → server shutdown sequence.
- **Edge cluster with failover**: Least-loaded node selection, automatic heartbeat-based failure detection, and cascading failover through healthy nodes.
- **Structured error responses**: `WorkerErrorDetail` provides request ID, worker name, runtime, node ID, and logs back to the caller — excellent for debugging.
- **Quota system with tiered plans**: Built-in usage tracking with daily upsert pattern and per-plan limits is production-ready design.
- **Security headers middleware**: HSTS, CSP, X-Frame-Options, Referrer-Policy all applied globally.

---

## Questions for Author

1. **Sandbox isolation**: Is there a plan to add OS-level isolation (containers, namespaces, seccomp) for worker execution, or is the subprocess model the intended long-term approach?
2. **Edge internal auth**: What's the threat model for inter-node communication? Are edge nodes assumed to be on a private network?
3. **Test strategy**: Is there a plan for test coverage? The Makefile has `test` and `lint` targets but no test files exist. What's the priority ordering for test additions?
4. **CORS configuration**: Is there a reason CORS allows all origins? Is this intended only for development?
5. **Worker name collision**: Worker names are globally unique (not per-user). Is this intentional? It means user A can claim a name before user B.

---

## Test Coverage Assessment

- [ ] Happy path tested
- [ ] Error cases tested
- [ ] Edge cases tested
- [ ] Integration tests present
- [ ] Security validation tested
- [ ] JWT/auth flow tested

**No test files exist.** Priority test areas:

1. `security.Validator` — bypass attempts with aliased imports, string splitting, edge cases
2. `service.ValidateJWT` / `generateJWT` — expiry, tampering, malformed tokens
3. `crypto.Encrypt` / `Decrypt` — round-trip, wrong key, corrupted data
4. `runtime.Pool` — concurrency, timeout, full pool rejection
5. `service.QuotaService` — limit enforcement, boundary conditions

---

## Checklist

- [ ] No security vulnerabilities (sandbox escape, hardcoded secrets, unauthenticated internal APIs)
- [ ] Performance is acceptable (payload-via-env-var size limit concern)
- [x] Code is readable
- [ ] Tests are adequate (zero tests)
- [x] Documentation is present (README, examples, config comments)
- [x] Architecture is sound
- [x] Error handling is consistent (with exceptions noted above)
