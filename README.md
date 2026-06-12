# be-go-template

Production-leaning Go backend template with layered architecture, replaceable interfaces, MongoDB, Redis cache/lock, JWT auth, refresh-token rotation, logging, standardized errors, Prometheus metrics, readiness checks, admin monitoring endpoints, seed, and versioned migrations.

## Architecture

Runtime direction:

```text
HTTP handler -> service interface -> repository interface -> database interface
database interface -> Mongo adapter + cache interface
cache interface -> Redis adapter
```

Important boundaries:

- Domain packages define entities and interfaces.
- `internal/app/*` implements application services.
- `internal/repository/mongo/*` implements repositories against `platform/database.Database`.
- Repositories do not coordinate Redis cache directly.
- `CachedDatabase` owns read-through cache, cache invalidation, and lock policy.
- Infrastructure dependencies are expressed as interfaces so tests can replace MongoDB, Redis, token, repository, and service implementations.

## Requirements

- Go 1.26+
- MongoDB
- Redis

## Configuration

All config is loaded from environment variables. Local defaults are intentionally runnable for development.

Start from [`.env.example`](.env.example) when you want a complete local environment file.

| Variable | Default |
| --- | --- |
| `APP_NAME` | `be-go-template` |
| `APP_ENV` | `local` |
| `HTTP_ADDR` | `:8080` |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,http://localhost:5173` |
| `LOG_LEVEL` | `info` |
| `LOG_FORMAT` | `json` |
| `LOG_TO_CONSOLE` | `true` |
| `LOG_TO_FILE` | `false` |
| `LOG_FILE_PATH` | `logs/app.log` |
| `LOG_MAX_SIZE_MB` | `100` |
| `LOG_MAX_BACKUPS` | `10` |
| `LOG_MAX_AGE_DAYS` | `30` |
| `LOG_COMPRESS` | `true` |
| `JWT_ACCESS_CURRENT_KEY` | `local/<base64-secret>` |
| `JWT_ACCESS_PREVIOUS_KEY` | empty |
| `JWT_ACCESS_PREVIOUS_NOT_AFTER` | empty |
| `JWT_ACCESS_TTL` | `15m` |
| `JWT_REFRESH_TTL` | `720h` |
| `MONGO_URI` | `mongodb://localhost:27017` |
| `MONGO_DATABASE` | `be_go_template` |
| `MONGO_MAX_POOL_SIZE` | `100` |
| `MONGO_MIN_POOL_SIZE` | `0` |
| `MONGO_CONNECT_TIMEOUT` | `10s` |
| `MONGO_READ_PREFERENCE` | `primary` |
| `REDIS_ADDR` | `localhost:6379` |
| `REDIS_PASSWORD` | empty |
| `REDIS_DB` | `0` |
| `REDIS_LOCK_PREFIX` | `lock:` |
| `REDIS_TLS_ENABLED` | `false` |
| `REDIS_TLS_SERVER_NAME` | empty |
| `AUTH_RATE_LIMIT_ENABLED` | `true` |
| `AUTH_RATE_LIMIT_LOGIN_PER_MINUTE` | `10` |
| `AUTH_RATE_LIMIT_REFRESH_PER_MINUTE` | `30` |
| `AUTH_RATE_LIMIT_REGISTER_PER_MINUTE` | `5` |
| `RATE_LIMIT_FALLBACK` | `allow` locally, `block` in production |
| `AUTH_LOCKOUT_MAX_FAILURES` | `5` |
| `AUTH_LOCKOUT_DURATION` | `15m` |
| `METRICS_ENABLED` | `true` |
| `METRICS_PATH` | `/metrics` |
| `READY_TIMEOUT` | `2s` |
| `READY_REQUIRES_REDIS` | `false` locally, `true` in production |
| `MONGO_DEGRADED_THRESHOLD` | `500ms` |
| `REDIS_DEGRADED_THRESHOLD` | `200ms` |

Production startup requires explicit `CORS_ALLOWED_ORIGINS`.

JWT key format:

```text
<key-id>/<base64-secret>
```

`JWT_ACCESS_CURRENT_KEY` signs new access tokens. `JWT_ACCESS_PREVIOUS_KEY` only validates older access tokens until `JWT_ACCESS_PREVIOUS_NOT_AFTER`.

## Commands

Run tests and static checks:

```sh
go test ./...
go vet ./...
```

Run versioned Mongo migrations:

```sh
go run ./cmd/migrate
```

Seed an admin user:

```sh
ADMIN_EMAIL=admin@example.com \
ADMIN_PASSWORD='change-me' \
ADMIN_NAME='Administrator' \
go run ./cmd/seed
```

Start the API:

```sh
go run ./cmd/api
```

Run the local stack with Docker Compose:

```sh
docker compose up --build api
```

Run migrations against the Compose MongoDB service:

```sh
docker compose --profile tools run --rm migrate
```

Seed an admin user through Compose:

```sh
ADMIN_EMAIL=admin@example.com \
ADMIN_PASSWORD='change-me' \
ADMIN_NAME='Administrator' \
docker compose --profile tools run --rm seed
```

## HTTP endpoints

Public:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /v1/auth/register`
- `POST /v1/auth/login`
- `POST /v1/auth/refresh`

Authenticated:

- `GET /v1/users/me`
- `POST /v1/auth/logout`
- `POST /v1/auth/logout-all`
- `GET /v1/auth/devices`
- `GET /v1/auth/login-history` with `limit`, `offset`, `cursor`

Admin:

- `GET /v1/admin/monitoring/status`
- `GET /v1/admin/monitoring/dependencies`
- `GET /v1/admin/monitoring/runtime`
- `GET /v1/admin/monitoring/auth-stats?from=&to=`
- `GET /v1/admin/monitoring/errors?limit=&offset=&cursor=&error_code=&request_id=&status=&from=&to=`
- `GET /v1/admin/monitoring/audit-logs?limit=&offset=&cursor=&actor_user_id=&action=&resource_type=&resource_id=&request_id=&from=&to=`

Protected endpoints require:

```text
Authorization: Bearer <access-token>
```

Device metadata for login is optional and used for UX/audit, not as a security lookup key:

```text
X-Device-ID: <client-device-id>
X-Device-Name: <human-readable-device-name>
```

## Auth model

- Access tokens are JWTs with `kid` support for key rotation.
- Refresh tokens are random opaque secrets stored as hashes.
- Refresh uses rotation: the old hash is replaced atomically with a new hash.
- Reuse detection revokes the token family when an active session has already rotated to a different refresh hash.
- Logout revokes the session and blacklists the access token.
- Blacklisted access token IDs are stored in Redis with TTL and backed by MongoDB `revoked_tokens` for Redis restart fallback.
- Login history and audit events are persisted for device visibility and admin monitoring.

## Observability

- Structured logging can write to terminal and file.
- Request logs include request id, trace id, span id, method, path, query, status, latency, ip, user agent, and user/session fields when available.
- Error responses use a stable envelope with `success`, `request_id`, and structured `error`.
- Prometheus metrics are exposed at `METRICS_PATH`.
- Admin monitoring endpoints expose runtime, dependency, auth, audit, and recent error views.

See [docs/operations.md](docs/operations.md) for dependency degradation behavior and operational contracts.

## Project docs

- [Implementation plan](docs/implementation-plan.md)
- [Backend template context](docs/contexts/backend-template/CONTEXT.md)
- [Operations](docs/operations.md)
