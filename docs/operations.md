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

## Cache policy

- `FindOne` may use read-through cache when a deterministic `ReadOptions.CacheKey` is provided.
- `FindMany` does not cache by default.
- `FindMany` may cache only when the caller provides an explicit deterministic cache key and the filter implements the cacheable-filter contract.
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

## Account lockout

Login failures are tracked as consecutive failed attempts on the user document.

- `AUTH_LOCKOUT_MAX_FAILURES` controls when the account is temporarily locked.
- `AUTH_LOCKOUT_DURATION` controls how long `locked_until` is set.
- A successful login resets `failed_login_attempts` and clears `locked_until`.
- Repository updates invalidate both `user:id:{id}` and `user:email:{email}` cache keys so auth decisions do not read stale lockout state.

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

## Logging contract

- Enable console logs with `LOG_TO_CONSOLE=true`.
- Enable file logs with `LOG_TO_FILE=true`, `LOG_FILE_PATH`, and rotation knobs `LOG_MAX_SIZE_MB`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS`, `LOG_COMPRESS`.
- Do not log passwords, refresh tokens, access tokens, JWT secrets, Redis passwords, or Mongo credentials.
- Request id is propagated through context and included in logs when available.
- Trace id and span id are propagated through context when the client provides them.
- Auth middleware adds user id, session id, and token id to context for downstream logging.

## Migration contract

`go run ./cmd/migrate` runs ordered migrations from `cmd/migrate`.

Each migration has:

- a monotonic string `Version`
- a human-readable `Name`
- an `Apply(ctx)` function

Applied versions are recorded in `schema_migrations` with a unique `version` index. A migration is recorded only after `Apply` succeeds.

## Outbox contract

Audit logs and HTTP error events are enqueued into `outbox_events` first. The API process starts a background worker that drains pending events and writes them into `audit_logs` or `error_events`.

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
