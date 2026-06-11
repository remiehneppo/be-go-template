# Backend Template Context

## Purpose

This repository is a production-leaning Go backend template. It is designed around explicit interfaces, replaceable infrastructure adapters, JWT authentication, database/cache coordination, logging, error handling, and monitoring.

## Core language

- Handler: HTTP adapter that parses requests and calls service interfaces.
- Service: application use-case implementation behind a domain interface.
- Repository: persistence boundary that uses the database abstraction only.
- Database abstraction: boundary that coordinates MongoDB and cache behavior, including read-through cache, invalidation, and lock policy.
- Cache abstraction: boundary for Redis-backed cache, locks, token blacklist, and rate limit primitives.
- Session: a user login instance tied to refresh token rotation and device metadata.
- Device metadata: UX and audit data about a login device, not a security lookup key.
- Outbox: durable retry queue for audit and operational events.

## Current checkpoint

The template now has the main runtime skeleton wired end-to-end:

- Gin API bootstrap with graceful shutdown.
- Typed environment configuration.
- Structured logging to terminal/file.
- Context key contract for request, trace, auth, and logger fields.
- Standard application errors and HTTP response envelope.
- MongoDB adapter, Redis cache adapter, cached database coordination, and Mongo-backed outbox worker.
- Domain interfaces for auth, user, monitoring, and shared pagination/validation.
- Mongo repositories for users, sessions, login history, audit logs, revoked tokens, error events, and monitoring stats.
- JWT access token service with key id rotation and refresh-token support.
- Auth register/login/refresh/logout/logout-all, device list, login history, and account lockout on repeated failed login.
- Redis-backed token blacklist with MongoDB fallback.
- Auth rate limiting with explicit fallback behavior.
- Prometheus HTTP metrics.
- Readiness dependency checks with healthy/degraded/unhealthy levels.
- Admin monitoring endpoints backed by final audit/error collections.
- Idempotent admin seed command.
- Versioned Mongo migration runner.
- Operations documentation for degradation behavior, logging, errors, migrations, and seed.
