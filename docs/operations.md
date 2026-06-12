# Operations

This document records runtime contracts that should stay explicit while the template evolves.

## Health and readiness

| Endpoint | Purpose | Dependency checks |
| --- | --- | --- |
| `GET /healthz` | Process liveness | None |
| `GET /readyz` | Serving readiness | MongoDB and Redis through the readiness checker |
| `GET /metrics` | Prometheus scrape endpoint | None beyond process availability |

Readiness uses `READY_TIMEOUT`. Dependency latency is classified with `MONGO_DEGRADED_THRESHOLD` and `REDIS_DEGRADED_THRESHOLD`.

Redis readiness is required when `READY_REQUIRES_REDIS=true`. Local defaults keep Redis optional for readiness; production defaults require it.

## Prometheus metrics

The API exposes Prometheus text format at `METRICS_PATH` when metrics are enabled.

- Default namespace: `be_go_template`
- Core metric families:
  - HTTP request counters/duration
  - cache event counters and lock duration
  - database dependency error counters
  - auth counters and active session gauge

Example scrape config:

```yaml
scrape_configs:
  - job_name: be-go-template-api
    metrics_path: /metrics
    static_configs:
      - targets:
          - be-go-template:8080
```

## Dependency degradation matrix

| Dependency state | Expected behavior |
| --- | --- |
| MongoDB down | API operations that require persistence return a dependency/server error. `/readyz` returns unavailable. |
| Redis cache get/set down | Cached reads fall back to MongoDB where possible. Cache failures are logged and should not fail normal reads. |
| Redis read lock down | `ReadOptions.LockOnMiss=true` falls back to direct MongoDB read and logs a warning. |
| Redis write lock down, `StrictLock=false` | Write continues against MongoDB and cache invalidation is best-effort. |
| Redis write lock down, `StrictLock=true` | Write fails with a dependency error. |
| Redis token blacklist down | Token validation falls back to MongoDB `revoked_tokens`. |
| Redis rate limiter down | Behavior follows `RATE_LIMIT_FALLBACK`: `allow` for local development, `block` by production default. |
| Outbox enqueue fails | The caller receives the append error; auth audit calls are best-effort and ignore it. |
| Outbox worker write fails | The event is marked failed and retried after `process_after`. |

Database calls enforce typed `ReadOptions` and `WriteOptions` before reaching MongoDB. Invalid combinations fail fast, and the lock fallback contract is defined by the option values rather than implicit repository behavior.

## Health levels

Readiness and monitoring use a shared `HealthLevel` vocabulary:

- `healthy`: dependency is available and latency is under the configured degraded threshold.
- `degraded`: dependency responds, but latency exceeds `MONGO_DEGRADED_THRESHOLD` or `REDIS_DEGRADED_THRESHOLD`.
- `unhealthy`: dependency is unavailable, not configured, or required but down.

Defaults:

- `MONGO_DEGRADED_THRESHOLD=500ms`
- `REDIS_DEGRADED_THRESHOLD=200ms`

## Cache policy

- `FindOne` may use read-through cache when a deterministic `ReadOptions.CacheKey` is provided.
- `FindMany` does not cache by default.
- `FindMany` may cache only when the caller provides an explicit deterministic cache key and the filter implements the cacheable-filter contract.
- If a `FindMany` call supplies `CacheKey` but the filter is not cacheable, non-production environments fail fast and production logs a warning then skips cache.
- Repositories should pass invalidation keys through database write options when they know affected cache entries.
- Repository code must not call Redis directly for query caching.

## Error contract

HTTP responses use this envelope:

```json
{
  "success": false,
  "request_id": "request-id",
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input",
    "details": [
      {
        "field": "email",
        "reason": "invalid_format"
      }
    ]
  }
}
```

Internal logs may include operation names and wrapped causes. Client responses must only expose safe messages.

Successful HTTP responses use the same wrapper shape with `success: true` and the payload in `data`. That includes auth, user, monitoring, `/healthz`, and `/readyz` responses.

Validation errors use a stable schema:

```json
{
  "success": false,
  "request_id": "request-id",
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input",
    "retryable": false,
    "details": [
      {
        "field": "body",
        "reason": "invalid_json",
        "meta": {
          "kind": "syntax"
        }
      }
    ]
  }
}
```

Structured validation details may also use `reason="invalid_type"` with `meta` fields such as `expected` when JSON types do not match the request shape.

Server-side dependency errors set `retryable: true` in the error envelope. Client-side validation and authorization errors set `retryable: false`.

Error code table:

| Code | HTTP status | Typical source |
| --- | --- | --- |
| `INTERNAL_ERROR` | 500 | Unknown/unwrapped server error |
| `VALIDATION_ERROR` | 400 | Input validation, bind, or schema checks |
| `UNAUTHORIZED` | 401 | Missing/invalid credentials |
| `FORBIDDEN` | 403 | Authenticated but not allowed |
| `NOT_FOUND` | 404 | Missing resource |
| `CONFLICT` | 409 | Duplicate key or state conflict |
| `TOKEN_EXPIRED` | 401 | Expired JWT or expired previous JWT key overlap |
| `TOKEN_REVOKED` | 401 | Logout or blacklist hit |
| `DEPENDENCY_ERROR` | 503 | Mongo/Redis/other dependency unavailable |
| `RATE_LIMITED` | 429 | Auth or endpoint rate limit exceeded |
| `REQUEST_TOO_LARGE` | 413 | Request body exceeds configured limit |
| `TIMEOUT` | 504 | Request timeout middleware or dependency deadline |

Internal `AppError` values wrap the underlying cause for logging and tracing. Client responses should only use the safe `message` and `details` fields.

## Auth rate limit

Auth endpoints use Redis-backed rate limiting with per-endpoint keys:

- `POST /v1/auth/register` by IP
- `POST /v1/auth/login` by IP plus email when available
- `POST /v1/auth/refresh` by IP

When the Redis limiter is unavailable, behavior follows `RATE_LIMIT_FALLBACK`.

## JWT key rotation

Access tokens carry a `kid` header and the validator accepts the configured current key plus one previous key during a bounded overlap window.

Rotation process:

1. Generate a new key pair and set it as `JWT_ACCESS_CURRENT_KEY`.
2. Move the old current key into `JWT_ACCESS_PREVIOUS_KEY`.
3. Set `JWT_ACCESS_PREVIOUS_NOT_AFTER` to the end of the overlap window.
4. Deploy the new config.
5. After the overlap window expires, remove the previous key from config.

Rules:

- `JWT_ACCESS_CURRENT_KEY` signs new tokens immediately after deploy.
- `JWT_ACCESS_PREVIOUS_KEY` only validates tokens issued before rotation.
- `JWT_ACCESS_PREVIOUS_NOT_AFTER` must be in the future while the previous key is configured.
- Once `JWT_ACCESS_PREVIOUS_NOT_AFTER` passes, the previous key is rejected even if the secret is still present.

## Account lockout

Login failures are tracked as consecutive failed attempts on the user document.

- `AUTH_LOCKOUT_MAX_FAILURES` controls when the account is temporarily locked.
- `AUTH_LOCKOUT_DURATION` controls how long `locked_until` is set.
- A successful login resets `failed_login_attempts` and clears `locked_until`.
- Repository updates invalidate both `user:id:{id}` and `user:email:{email}` cache keys so auth decisions do not read stale lockout state.

## Refresh token binding

Session records keep `ip`, `user_agent`, and `device_id` for audit and device history.

- `DeviceID` is validated as UUID v4 when the client sends it.
- If the client omits `DeviceID`, the server generates one.
- `DeviceID` is for UX and audit only; refresh and session lookup never use it as a security key.
- `AUTH_REFRESH_IP_ANOMALY_ACTION=audit` logs and audits refreshes from a new IP.
- `AUTH_REFRESH_IP_ANOMALY_ACTION=revoke` revokes the session family on refresh IP mismatch.

## Refresh rotation and logout invalidation

Refresh rotation is atomic at the session row level.

- The server hashes the presented refresh token and rotates it against the stored session hash.
- If rotation fails because the hash no longer matches, the server reads the active session again.
- If the session is still active, the request is treated as a reuse/race condition and the full token family is revoked.
- If the session is already inactive or missing, the request is treated as a normal invalid refresh token and the family is not revoked again.
- Logout invalidates the current session and blacklists the current access token ID.
- Logout-all invalidates all active sessions for the user and clears session/device-list caches through repository invalidation keys.

## User profile cache validation

`GET /v1/users/me` returns an `ETag` header based on the safe response payload. A matching `If-None-Match` returns `304 Not Modified`.

## Monitoring list filters

`GET /v1/admin/monitoring/errors` accepts:

- `limit`
- `offset`
- `cursor`
- `error_code`
- `request_id`
- `status`
- `from`
- `to`

`GET /v1/admin/monitoring/audit-logs` accepts:

- `limit`
- `offset`
- `cursor`
- `actor_user_id`
- `action`
- `resource_type`
- `resource_id`
- `request_id`
- `from`
- `to`

## Admin monitoring access

Admin monitoring endpoints are protected by both authentication and admin-role authorization.

- All `/v1/admin/*` routes require a valid bearer access token.
- Allowed roles come from `MONITORING_ADMIN_ROLES`; the default local/admin role is `admin`.
- `AdminGuard` rejects users without one of the configured roles with `403 Forbidden`.
- Monitoring endpoints are intended for the admin panel, not for general application clients.

Endpoint groups:

- `GET /v1/admin/monitoring/status` returns service status and deployment identity.
- `GET /v1/admin/monitoring/dependencies` returns Mongo/Redis readiness health.
- `GET /v1/admin/monitoring/runtime` returns runtime process metrics.
- `GET /v1/admin/monitoring/auth-stats` returns login/logout/refresh counters within the requested time range.
- `GET /v1/admin/monitoring/errors` returns recent error events with filter support.
- `GET /v1/admin/monitoring/audit-logs` returns audit events with filter support.

## Logging contract

- Enable console logs with `LOG_TO_CONSOLE=true`.
- Enable file logs with `LOG_TO_FILE=true`, `LOG_FILE_PATH`, and rotation knobs `LOG_MAX_SIZE_MB`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS`, `LOG_COMPRESS`.
- Do not log passwords, refresh tokens, access tokens, JWT secrets, Redis passwords, or Mongo credentials.
- Request id is propagated through context and included in logs when available.
- Trace id and span id are propagated through context when the client provides them.
- Auth middleware adds user id, session id, and token id to context for downstream logging.

## Context propagation

The template uses `internal/platform/ctxkeys` for all request-scoped context values.

- Keys are stable and centralized: `request_id`, `user_id`, `session_id`, `token_id`, `roles`, `trace_id`, `span_id`, `logger`, `request_started_at`.
- Middleware should set context values through these keys instead of raw strings.
- Services and repositories should read request-scoped metadata through the same keys to keep logging and tracing consistent.

## Migration contract

`go run ./cmd/migrate` runs ordered migrations from `cmd/migrate`.

Each migration has:

- a monotonic string `Version`
- a human-readable `Name`
- an `Apply(ctx)` function

Applied versions are recorded in `schema_migrations` with a unique `version` index. A migration is recorded only after `Apply` succeeds.

Migration strategy:

- Use `cmd/migrate` for schema evolution, backfills, and index changes.
- Keep migrations idempotent so reruns after partial failure are safe.
- Prefer monotonic string versions with a timestamp prefix, so sort order matches rollout order.
- Put destructive or data-shaping changes in explicit migrations, not in API startup code.
- Treat `bootstrap_indexes` as the first migration; it creates the canonical index set and then records its version.
- If a migration fails, the runner stops and does not record the version.
- `cmd/migrate` prints the number of applied migrations and the version/name of each applied item.

## Outbox contract

Audit logs, login history fallbacks, and HTTP error events are enqueued into `outbox_events` when the direct write path fails or when the async path is enabled. The API process starts a background worker that drains pending events and writes them into `audit_logs`, `login_history`, or `error_events`.

Outbox events use:

- `idempotency_key` unique index to prevent duplicate business events
- `status` values: `pending`, `processing`, `done`, `failed`
- `process_after` for retry scheduling
- `retry_count` and `max_retries` for bounded retry

Outbox runtime is configurable with:

- `OUTBOX_ENABLED`
- `OUTBOX_DRAIN_INTERVAL`
- `OUTBOX_BATCH_SIZE`
- `OUTBOX_MAX_RETRIES`
- `OUTBOX_RETRY_DELAY`

Outbox processing is at-least-once, not exactly-once.

- The worker treats duplicate-key conflicts from the target repository as success.
- Side-effect handlers must remain idempotent for the same logical payload.
- Duplicate outbox inserts are prevented by `idempotency_key`; duplicate downstream writes are ignored when they hit the same Mongo document key.

## Mongo transaction policy

The template does not rely on MongoDB multi-document transactions for the core auth flow.

- Direct writes are kept within a single repository boundary where possible.
- Cross-collection side effects use outbox retry instead of transaction coupling.
- If a deployment wants atomic multi-document writes, it must add transaction support explicitly at the infrastructure boundary.

Admin monitoring reads from the final `audit_logs` and `error_events` collections, not from the queue.

## Admin seed contract

`go run ./cmd/seed` is idempotent.

- If the user does not exist, it creates an admin user.
- If the user exists without the admin role, it grants the admin role.
- If the user already has the admin role, it performs no write.

Required variables:

- `ADMIN_EMAIL`
- `ADMIN_PASSWORD`

Optional:

- `ADMIN_NAME`

Seed output is explicit about whether the admin user was created, updated, or already present. The command is intended for bootstrap and local development, not for public registration flows.
