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

The first implementation checkpoint establishes bootstrap scaffolding, typed configuration, context key contracts, and logging primitives. Infrastructure adapters are implemented in later checkpoints.
