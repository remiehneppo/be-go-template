# Go Backend Template Implementation Plan

## 1. Mục tiêu

Xây dựng backend template bằng Go cho module `github.com/remihneppo/be-go-template`, dùng:

- Gin cho HTTP API.
- MongoDB làm database mặc định.
- Redis làm cache, distributed lock, token blacklist.
- Layered architecture: handler -> service -> repository -> database abstraction.
- Tất cả boundary chính giao tiếp qua interface để dễ thay implementation/mock khi test.
- Repository chỉ thao tác với database abstraction, không tự xử lý cache.
- Database abstraction chịu trách nhiệm phối hợp MongoDB + Redis để tối ưu read/write, giảm cache stampede, và tránh racing đọc/ghi dữ liệu.
- Auth production-leaning với JWT access token, refresh token rotation, device/session management, login history, logout invalidation.
- Logging đầy đủ ra terminal và file để debug request flow, trace error, và audit hành vi quan trọng.
- Error handling chuẩn hóa toàn hệ thống, có error code, wrapping, request id, stack/cause nội bộ, và response an toàn cho client.
- Monitoring service riêng để cung cấp dữ liệu vận hành cho admin panel trong tương lai.
- Prometheus metrics dùng client library chính thức, không tự implement text format.
- Outbox pattern cho audit/error event để giảm mất event khi transaction không available.

## 2. Kiến trúc tổng quan

### 2.1. Cấu trúc thư mục cần tạo

```text
cmd/api/
  main.go

cmd/seed/
  main.go

cmd/migrate/
  main.go

internal/config/
  config.go

internal/platform/logger/
  logger.go
  file_writer.go

internal/platform/ctxkeys/
  keys.go

internal/platform/errors/
  errors.go
  codes.go
  mapper.go

internal/platform/metrics/
  collector.go
  prometheus.go
  runtime.go

internal/platform/outbox/
  outbox.go
  mongo_outbox.go
  worker.go

internal/platform/cache/
  cache.go
  redis.go

internal/platform/ratelimit/
  limiter.go
  redis_limiter.go

internal/platform/database/
  database.go
  mongo.go
  cached_database.go
  options.go

internal/domain/user/
  entity.go
  repository.go
  service.go

internal/domain/auth/
  entity.go
  repository.go
  service.go
  token.go

internal/domain/monitoring/
  entity.go
  repository.go
  service.go

internal/domain/common/
  pagination.go
  validation.go

internal/repository/mongo/
  user_repository.go
  session_repository.go
  login_history_repository.go
  audit_log_repository.go

internal/app/auth/
  auth_service.go
  token_service.go

internal/app/monitoring/
  monitoring_service.go

internal/app/outbox/
  worker.go

internal/handler/http/
  router.go
  response.go
  auth_handler.go
  user_handler.go
  monitoring_handler.go

internal/middleware/
  auth.go
  admin.go
  request_id.go
  recovery.go
  logging.go
  error_handler.go
  prometheus.go

internal/bootstrap/
  app.go
  indexes.go

tests/
  integration/
```

### 2.2. Dependency direction

- `handler` chỉ phụ thuộc service interface.
- `service` chỉ phụ thuộc repository/token/cache interface khi thật sự cần.
- `repository` chỉ phụ thuộc database abstraction.
- `database` adapter phụ thuộc MongoDB driver và cache interface.
- `cache` adapter phụ thuộc Redis client.
- `monitoring` service đọc từ repository/log/metrics abstraction, không đọc trực tiếp từ Gin, Mongo client, hoặc Redis client.
- Domain package không phụ thuộc framework, database driver, Redis client, Gin, hoặc config runtime.
- `internal/domain/*` chứa entity và interface; `internal/app/*` chứa implementation use-case/service để tránh nhầm với domain service contract.

## 3. Interface contract cần có

### 3.1. Cache abstraction

Checklist:

- [ ] Tạo `internal/platform/cache.Cache` interface.
- [ ] Thêm method `Get(ctx, key, dest) error`.
- [ ] Thêm method `Set(ctx, key, value, ttl) error`.
- [ ] Thêm method `Delete(ctx, keys ...string) error`.
- [ ] Thêm method `Exists(ctx, key) (bool, error)`.
- [ ] Thêm method `WithLock(ctx, key, ttl, fn) error`.
- [ ] Thêm method `Close() error`.
- [ ] Chuẩn hóa lỗi `ErrCacheMiss`.
- [ ] Implement Redis adapter.
- [ ] Redis adapter encode/decode JSON cho object cache.
- [ ] Redis lock dùng `SET NX PX` và release an toàn bằng value owner token.
- [ ] Unit test cache mock contract.

### 3.2. Database abstraction

Checklist:

- [ ] Tạo `internal/platform/database.Database` interface.
- [ ] Thêm `FindOne(ctx, collection string, filter any, dest any, opts ReadOptions) error`.
- [ ] Thêm `FindMany(ctx, collection string, filter any, dest any, opts ReadOptions) error`.
- [ ] Thêm `InsertOne(ctx, collection string, document any, opts WriteOptions) error`.
- [ ] Thêm `UpdateOne(ctx, collection string, filter any, update any, opts WriteOptions) error`.
- [ ] Thêm `DeleteOne(ctx, collection string, filter any, opts WriteOptions) error`.
- [ ] Không dùng variadic option mơ hồ cho behavior quan trọng; dùng typed options `ReadOptions` và `WriteOptions` có zero-value an toàn.
- [ ] `ReadOptions` gồm `CacheKey`, `CacheTTL`, `LockOnMiss`.
- [ ] `WriteOptions` gồm `LockKey`, `InvalidateKeys`, `StrictLock`.
- [x] Validate options trước khi chạy query; option invalid phải log warning hoặc trả lỗi theo environment policy.
- [ ] Implement `MongoDatabase` chỉ xử lý MongoDB.
- [ ] Implement `CachedDatabase` wrap `MongoDatabase` và `Cache`.
- [ ] Repository luôn nhận `database.Database`, không nhận Mongo client trực tiếp.
- [ ] Database abstraction chịu trách nhiệm read-through cache.
- [ ] Database abstraction chịu trách nhiệm invalidation sau write.
- [ ] Database abstraction chỉ dùng Redis lock khi `ReadOptions.LockOnMiss=true`.
- [ ] Nếu read lock fail và `LockOnMiss=true`, fallback mặc định là đọc thẳng MongoDB, log warning, không fail API.
- [ ] Database abstraction dùng write lock/invalidation để tránh stale read sau write.
- [ ] Nếu write lock fail và `WriteOptions.StrictLock=true`, write fail với `DEPENDENCY_ERROR`; nếu `StrictLock=false`, ghi MongoDB rồi invalidate best-effort.
- [x] `FindMany` chỉ cache khi filter implement `CacheableFilter` và caller truyền `CacheKey` deterministic.
- [x] Nếu `FindMany` có `CacheKey` nhưng filter không implement `CacheableFilter`, skip cache và log warning ở production; có thể fail/panic trong dev/test theo config.
- [x] Unit test cached database với mock base database và mock cache.

### 3.3. Repository interfaces

Checklist:

- [ ] Tạo `UserRepository` trong domain.
- [ ] `UserRepository.Create(ctx, user) error`.
- [ ] `UserRepository.FindByID(ctx, id) (*User, error)`.
- [ ] `UserRepository.FindByEmail(ctx, email) (*User, error)`.
- [ ] `UserRepository.UpdateLastLogin(ctx, userID, at) error`.
- [ ] Tạo `SessionRepository` trong domain auth.
- [ ] `SessionRepository.Create(ctx, session) error`.
- [ ] `SessionRepository.FindActiveByID(ctx, sessionID) (*Session, error)`.
- [ ] `SessionRepository.FindByRefreshTokenHash(ctx, hash) (*Session, error)`.
- [ ] `SessionRepository.RotateRefreshToken(ctx, sessionID, oldHash, newHash, expiresAt) error`.
- [ ] `SessionRepository.Revoke(ctx, sessionID, reason, revokedAt) error`.
- [ ] `SessionRepository.RevokeAllByUserID(ctx, userID, reason, revokedAt) error`.
- [ ] `SessionRepository.ListActiveByUserID(ctx, userID) ([]Session, error)`.
- [ ] Tạo `LoginHistoryRepository`.
- [ ] `LoginHistoryRepository.Append(ctx, event) error`.
- [ ] `LoginHistoryRepository.ListByUserID(ctx, userID, limit, offset) ([]LoginHistory, error)`.

### 3.4. Service interfaces

Checklist:

- [ ] Tạo `AuthService` interface.
- [ ] `Register(ctx, input) (*AuthResult, error)`.
- [ ] `Login(ctx, input, meta) (*AuthResult, error)`.
- [ ] `Refresh(ctx, refreshToken, meta) (*AuthResult, error)`.
- [ ] `Logout(ctx, accessToken, sessionID) error`.
- [ ] `LogoutAll(ctx, userID) error`.
- [ ] `ListDevices(ctx, userID) ([]DeviceSession, error)`.
- [ ] `ListLoginHistory(ctx, userID, pagination) ([]LoginHistory, error)`.
- [ ] Tạo `TokenService` interface.
- [ ] `GenerateAccessToken(ctx, claims) (string, time.Time, error)`.
- [ ] `ValidateAccessToken(ctx, token) (*AccessClaims, error)`.
- [ ] `GenerateRefreshToken() (plain string, hash string, error)`.
- [ ] `HashRefreshToken(plain string) string`.
- [ ] `BlacklistAccessToken(ctx, tokenID, ttl) error`.
- [ ] `IsAccessTokenBlacklisted(ctx, tokenID) (bool, error)`.

### 3.5. Logger abstraction

Checklist:

- [x] Tạo `internal/platform/logger.Logger` interface.
- [x] Hỗ trợ level `debug`, `info`, `warn`, `error`.
- [x] Hỗ trợ structured fields: `request_id`, `user_id`, `session_id`, `ip`, `method`, `path`, `status`, `latency_ms`, `error_code`.
- [x] Hỗ trợ `With(fields...) Logger` để truyền context qua layer.
- [x] Hỗ trợ `WithContext(ctx, logger)` và `FromContext(ctx)` để carry `request_id`, `user_id`, `session_id` qua service/repository.
- [x] Ghi log ra terminal.
- [x] Ghi log ra file.
- [x] File log có cấu hình path, max size, max age, max backups, compress rotation.
- [ ] Tách access log và application/error log nếu config bật.
- [x] Không log password, refresh token, access token, secret.
- [x] Có helper mask/redact field nhạy cảm.
- [x] Unit test logger không ghi secret/token ra output.
- [x] Unit test `ContextLogger` giữ được request fields khi đi qua nhiều layer.

### 3.6. Context propagation contract

Checklist:

- [x] Tạo package `internal/platform/ctxkeys` để định nghĩa toàn bộ context key tập trung.
- [x] Dùng unexported/custom key type, không dùng raw string key trực tiếp trong middleware/service.
- [x] Key tối thiểu: `request_id`, `user_id`, `session_id`, `token_id`, `trace_id`, `span_id`, `logger`.
- [x] Middleware request id set `request_id` và logger vào context.
- [x] Auth middleware bổ sung `user_id`, `session_id`, `token_id` vào context.
- [x] Middleware tracing-readiness đọc `X-Trace-ID` nếu có, validate format, nếu không có thì có thể dùng request id làm trace id tạm.
- [x] Logger lấy các field chuẩn từ context, không để từng layer tự đặt key riêng.

### 3.7. Monitoring service interfaces

Checklist:

- [ ] Tạo `MonitoringService` interface.
- [ ] `GetSystemStatus(ctx) (*SystemStatus, error)`.
- [ ] `GetRuntimeMetrics(ctx) (*RuntimeMetrics, error)`.
- [ ] `GetDependencyStatus(ctx) (*DependencyStatus, error)`.
- [ ] `GetAuthStats(ctx, from, to) (*AuthStats, error)`.
- [ ] `GetRecentErrors(ctx, pagination) ([]ErrorEvent, error)`.
- [ ] `GetRecentAuditLogs(ctx, pagination) ([]AuditLog, error)`.
- [ ] Tạo `AuditLogRepository`.
- [ ] `AuditLogRepository.Append(ctx, event) error`.
- [ ] `AuditLogRepository.List(ctx, filter, pagination) ([]AuditLog, error)`.
- [ ] Monitoring service chỉ dùng abstraction/repository, không gọi Mongo/Redis client trực tiếp.


### 3.8. Outbox và token revocation interfaces

Checklist:

- [ ] Tạo `Outbox` interface: `Enqueue(ctx, event) error`, `ClaimBatch(ctx, limit) ([]Event, error)`, `MarkDone(ctx, id) error`, `MarkFailed(ctx, id, reason) error`.
- [ ] `OutboxEvent` gồm `ID`, `IdempotencyKey`, `Type`, `Payload`, `MaxRetries`, `RetryCount`, `Status`, `ProcessAfter`, `CreatedAt`, `UpdatedAt`.
- [ ] Caller sinh `ID`; với event có business dedup phải sinh `IdempotencyKey` deterministic, ví dụ `login:{user_id}:{request_id}` hoặc `audit:{request_id}:{action}`.
- [ ] Mongo unique index trên `idempotency_key` để retry ở layer trên không tạo duplicate event.
- [x] Worker xử lý idempotent theo `IdempotencyKey`/event target, không chỉ theo Mongo `_id`.
- [x] Implement `MongoOutbox` ghi event vào collection `outbox_events`.
- [x] Implement background worker drain outbox theo interval và retry với backoff.
- [x] Outbox event hỗ trợ tối thiểu audit log, error event, login history async fallback.
- [ ] Tạo `RevokedTokenRepository` lưu `jti`, `user_id`, `session_id`, `expires_at`, `revoked_at`.
- [ ] `RevokedTokenRepository` có TTL index theo `expires_at`.
- [ ] Token blacklist check Redis trước, nếu miss thì fallback Mongo `revoked_tokens`.
- [ ] Khi Redis recover, có thể warm lại blacklist từ Mongo theo token chưa expired.

## 4. Domain model

### 4.1. User

Checklist:

- [ ] Tạo `User` entity gồm `ID`, `Email`, `PasswordHash`, `Name`, `Roles`, `Status`, `FailedLoginAttempts`, `LockedUntil`, `CreatedAt`, `UpdatedAt`, `LastLoginAt`.
- [ ] Email unique, normalized lowercase.
- [ ] Password không bao giờ lưu plain text.
- [ ] User roles hỗ trợ tối thiểu `user`, `admin`.
- [ ] User role mặc định khi register là `user`; `admin` chỉ được seed/cấp qua admin path riêng, không qua public register.
- [ ] User status hỗ trợ tối thiểu `active`, `disabled`.
- [ ] Mongo index unique cho `email`.

### 4.2. Session/device

Checklist:

- [ ] Tạo `Session` entity gồm `ID`, `UserID`, `RefreshTokenHash`, `RefreshTokenExpiresAt`, `DeviceID`, `DeviceName`, `UserAgent`, `IP`, `TokenFamilyID`, `RevokedAt`, `RevokedReason`, `LastSeenAt`, `CreatedAt`, `UpdatedAt`.
- [ ] `DeviceID` lấy từ header/body nếu client gửi, nếu không server sinh UUID.
- [ ] `DeviceID` chỉ là hint phục vụ UX/hiển thị thiết bị, không dùng làm lookup key bảo mật.
- [ ] Validate `DeviceID` là UUID v4 hoặc server-generated UUID; reject/truncate input quá dài.
- [ ] Session lookup bảo mật luôn dựa trên `session_id` + refresh token hash hoặc access token claims, không dựa trên `device_id`.
- [ ] Một login tạo một session mới.
- [ ] Refresh token rotation cập nhật hash mới và expiry mới trên session hiện tại.
- [ ] `TokenFamilyID` sinh một lần khi login tạo session mới và bất biến trong toàn bộ refresh-token chain của session đó.
- [ ] `RotateRefreshToken` phải atomic theo điều kiện `session_id + old_refresh_hash + revoked_at nil + expires_at > now`.
- [ ] Nếu atomic update không match document nào, đọc lại session để phân biệt invalid/reuse/race/logout.
- [ ] Nếu session active nhưng `RefreshTokenHash` đã khác old hash, coi là stale token/reuse sau rotation và revoke toàn bộ session cùng `TokenFamilyID`.
- [ ] Nếu session đã revoked do logout, trả token invalid/revoked bình thường, không coi là reuse attack và không revoke thêm cả family.
- [ ] Nếu session expired, trả token expired, không coi là reuse attack.
- [ ] Refresh bằng token cũ sau rotation phải fail.
- [ ] Rotation race hai request dùng cùng refresh token: một request thắng atomic update, request còn lại bị stale old hash; policy mặc định revoke family để bảo thủ, nhưng phải audit `refresh_reuse_suspected`.
- [ ] Logout set `RevokedAt` và `RevokedReason`.
- [ ] Logout all revoke mọi session active theo user.
- [ ] Mongo index cho `user_id`, `refresh_token_hash`, `device_id`, `revoked_at`.

### 4.3. Login history

Checklist:

- [ ] Tạo `LoginHistory` entity gồm `ID`, `UserID`, `Email`, `Success`, `FailureReason`, `IP`, `UserAgent`, `DeviceID`, `CreatedAt`.
- [ ] Ghi history cho login thành công.
- [ ] Ghi history cho login thất bại khi xác định được email.
- [ ] Không ghi password hoặc token vào history.
- [ ] Mongo index cho `user_id`, `email`, `created_at`.

### 4.4. Audit log và error event

Checklist:

- [ ] Tạo `AuditLog` entity gồm `ID`, `ActorUserID`, `Action`, `ResourceType`, `ResourceID`, `IP`, `UserAgent`, `RequestID`, `Metadata`, `CreatedAt`.
- [ ] `AuditLog.Metadata` mặc định dùng `map[string]string` cho phase đầu để dễ redact/index; nếu cần payload phức tạp thì tạo typed metadata riêng theo action.
- [ ] Ghi audit log cho login, logout, logout all, refresh token reuse failure, user disabled/forbidden access.
- [ ] Không lưu token/password/secret trong metadata.
- [ ] Tạo `ErrorEvent` view/model cho monitoring gồm `RequestID`, `ErrorCode`, `Message`, `Cause`, `Stack`, `Path`, `Method`, `Status`, `UserID`, `CreatedAt`.
- [ ] Error event có thể lấy từ log file parser hoặc repository tùy implementation phase đầu tiên; mặc định phase đầu lưu event quan trọng vào Mongo qua audit/error repository.
- [x] Nếu audit/error/history write trực tiếp fail, enqueue vào outbox để retry thay vì mất event vĩnh viễn.
- [ ] Mongo index cho `request_id`, `action`, `actor_user_id`, `created_at`, `error_code`.

## 5. Auth API

### 5.1. Public endpoints

Checklist:

- [ ] `POST /v1/auth/register`.
- [ ] Validate email, password, name.
- [x] Hash password bằng bcrypt.
- [ ] Trả access token, refresh token, user summary, session/device info.
- [ ] Không trả refresh token hash, access token jti nội bộ, hoặc thông tin device của user khác.
- [ ] `POST /v1/auth/login`.
- [ ] Validate credentials.
- [ ] Ghi login history.
- [ ] Tạo session mới.
- [ ] Trả access token và refresh token.
- [ ] `POST /v1/auth/refresh`.
- [ ] Validate refresh token hash.
- [ ] Kiểm tra session chưa revoked và chưa expired.
- [ ] Rotate refresh token.
- [ ] Trả access token mới và refresh token mới.

### 5.2. Protected endpoints

Checklist:

- [ ] `POST /v1/auth/logout`.
- [ ] Lấy access token hiện tại.
- [ ] Blacklist access token đến hết TTL còn lại.
- [ ] Revoke session hiện tại.
- [ ] `POST /v1/auth/logout-all`.
- [ ] Revoke mọi session của user hiện tại.
- [ ] Blacklist access token hiện tại.
- [ ] `GET /v1/auth/devices`.
- [ ] Trả danh sách session active của user.
- [ ] Hỗ trợ `ETag`/`If-None-Match`; trả `304 Not Modified` nếu device list không đổi.
- [x] `GET /v1/auth/login-history`.
- [x] Trả lịch sử login có pagination.
- [ ] `GET /v1/users/me`.
- [ ] Trả user hiện tại.
- [ ] Hỗ trợ `ETag`/`If-None-Match`; trả `304 Not Modified` nếu profile không đổi.

### 5.3. Auth middleware

Checklist:

- [ ] Parse `Authorization: Bearer <token>`.
- [ ] Validate JWT signature và expiry.
- [ ] Validate `jti`, `sub`, `session_id`.
- [ ] Kiểm tra `jti` có nằm trong Redis blacklist không.
- [x] Kiểm tra session active qua repository/service hoặc database abstraction.
- [ ] Inject `user_id`, `session_id`, `token_id` vào Gin context.
- [ ] Trả lỗi JSON thống nhất cho unauthenticated/forbidden.

### 5.4. Health/readiness endpoints

Checklist:

- [x] `GET /healthz` chỉ kiểm tra process HTTP còn sống, không ping dependency nặng.
- [x] `GET /readyz` ping MongoDB và Redis với timeout mặc định 2 giây.
- [x] `/readyz` trả `503` nếu MongoDB không sẵn sàng.
- [x] Redis down làm `/readyz` degraded hoặc fail theo config `READY_REQUIRES_REDIS`; default local có thể degraded, production nên explicit.

### 5.5. Monitoring endpoints

Checklist:

- [ ] `GET /v1/admin/monitoring/status`.
- [ ] Trả trạng thái app, uptime, version, environment.
- [ ] `GET /v1/admin/monitoring/dependencies`.
- [ ] Trả trạng thái MongoDB, Redis, cache wrapper.
- [ ] `GET /v1/admin/monitoring/runtime`.
- [ ] Trả goroutines, memory, GC, CPU nếu có collector hỗ trợ.
- [ ] `GET /v1/admin/monitoring/auth-stats`.
- [ ] Trả số login success/failure, active sessions, revoked sessions theo khoảng thời gian.
- [ ] `GET /v1/admin/monitoring/errors`.
- [ ] Trả lỗi gần đây, filter theo `error_code`, `status`, `request_id`.
- [ ] `GET /v1/admin/monitoring/audit-logs`.
- [ ] Trả audit log gần đây, filter theo actor/action/resource/time.
- [ ] Toàn bộ `/v1/admin/*` yêu cầu auth middleware.
- [ ] Toàn bộ `/v1/admin/*` yêu cầu admin guard dựa trên `roles` của user.
- [ ] `GET /metrics` expose Prometheus metrics, có thể public nội bộ hoặc bảo vệ bằng config.

## 6. Database/cache consistency plan

Checklist:

- [ ] Quy ước cache key tập trung trong database/repository option helper.
- [ ] User by id key: `user:id:{id}`.
- [ ] User by email key: `user:email:{email}`.
- [ ] Session by id key: `session:id:{id}`.
- [ ] Session by refresh hash key: `session:refresh:{hash}`.
- [ ] Active devices key: `session:user:{user_id}:active`.
- [ ] Read path:
  - [ ] Nếu có `CacheKey`, đọc cache trước.
  - [ ] Cache miss chỉ acquire lock `lock:{CacheKey}` nếu `ReadOptions.LockOnMiss=true`.
  - [ ] Sau khi có lock, kiểm tra cache lại.
  - [ ] Nếu lock fail theo fallback policy hoặc vẫn miss sau lock, đọc MongoDB.
  - [ ] Set cache với TTL.
  - [ ] Release lock.
- [x] `FindMany` không cache mặc định.
- [x] `FindMany` chỉ được cache khi caller truyền `CacheKey` tường minh và filter implement `CacheableFilter` với normalized/deterministic representation.
- [ ] Không tự sinh cache key từ raw filter phức tạp.
- [ ] Write path:
  - [ ] Nếu có `LockKey`, acquire write lock.
  - [ ] Ghi MongoDB trước.
  - [ ] Invalidate `InvalidateKeys`.
  - [ ] Release lock.
- [ ] Khi cache lỗi, không làm API fail nếu MongoDB vẫn đọc/ghi được, trừ lock critical path được cấu hình strict.
- [ ] Với write nhạy cảm như refresh token rotation, dùng conditional atomic update thay vì read-then-write.
- [ ] Repository chịu trách nhiệm truyền `InvalidateKeys` cụ thể cho từng write operation.
- [ ] Khi refresh token rotation, repository truyền invalidate keys tối thiểu: `session:id:{id}`, `session:refresh:{old_hash}`, `session:user:{user_id}:active`.
- [ ] Sau rotation thành công, cache key `session:refresh:{new_hash}` chỉ được populate từ lần đọc tiếp theo hoặc set có kiểm soát.
- [ ] Register/login có nhiều write thì dùng Mongo transaction nếu deployment hỗ trợ replica set.
- [ ] Nếu transaction không available, audit/history write trực tiếp là best-effort và không làm fail auth chính.
- [x] Nếu audit/history/error write trực tiếp fail, enqueue outbox event để retry at-least-once.
- [ ] Outbox worker idempotent theo `IdempotencyKey`/business target để tránh duplicate side effects.
- [ ] Database abstraction không expose Mongo client/collection ra repository.
- [ ] Nếu repository dùng `bson.M`, giới hạn trong package `internal/repository/mongo`, không leak vào domain/service/handler.
- [ ] Log warning cho cache error.
- [ ] TTL mặc định:
  - [ ] User profile: 5-15 phút.
  - [ ] Session active: 1-5 phút.
  - [ ] Device list: 30-120 giây.
  - [ ] Access token blacklist: đúng thời gian còn lại của token.

### 6.1. Graceful degradation matrix

| Dependency down | Impact | Behavior mặc định | Config liên quan |
|---|---|---|---|
| Redis cache down | Cache miss, no read lock | Skip cache/lock, đọc MongoDB, log warning, metrics dependency error | `CACHE_FALLBACK_ALLOW=true` |
| Redis lock down trên read | Không chống stampede được | Nếu `LockOnMiss=true`, đọc MongoDB trực tiếp, không fail API | `READ_LOCK_STRICT=false` |
| Redis lock down trên write | Không có write lock | Nếu `StrictLock=true` thì fail write; nếu false thì ghi MongoDB và invalidate best-effort | per-call `WriteOptions.StrictLock` |
| Redis blacklist down/miss | Không chắc token đã logout chưa | Check Mongo `revoked_tokens`; nếu Mongo check fail thì trả dependency/auth error theo policy | `TOKEN_REVOCATION_FALLBACK_REQUIRED=true` |
| Redis rate limit down | Không rate limit được | Explicit config `RATE_LIMIT_FALLBACK=block|allow`; production default nên `block`, local default có thể `allow` | `RATE_LIMIT_FALLBACK` |
| MongoDB down | Không đọc/ghi source of truth | Return `503 DEPENDENCY_ERROR`, không che giấu bằng cache cho write/auth critical path | Mongo timeout config |
| Outbox MongoDB/write down | Audit/history/error retry không ghi được | Log error; auth chính không fail nếu event không critical | `OUTBOX_REQUIRED_FOR_AUTH=false` |


## 7. Configuration

Checklist:

- [x] `.env.example` gồm `APP_ENV`, `HTTP_ADDR`, `MONGO_URI`, `MONGO_DATABASE`, `REDIS_ADDR`, `REDIS_PASSWORD`, `JWT_ACCESS_SECRET_CURRENT`, `JWT_ACCESS_SECRET_PREVIOUS`, `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL`, `BCRYPT_COST`.
- [ ] Thêm Mongo config: `MONGO_MAX_POOL_SIZE`, `MONGO_MIN_POOL_SIZE`, `MONGO_CONNECT_TIMEOUT`, `MONGO_READ_PREFERENCE`.
- [x] Thêm Redis TLS config: `REDIS_TLS_ENABLED`, `REDIS_TLS_CA_CERT`, `REDIS_TLS_SERVER_NAME`.
- [x] Thêm HTTP safety config: `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT`, `HTTP_BODY_LIMIT_BYTES`, `ROUTE_TIMEOUT_DEFAULT`.
- [x] Thêm CORS config: `CORS_ALLOWED_ORIGINS`, `CORS_ALLOWED_METHODS`, `CORS_ALLOWED_HEADERS`.
- [x] Thêm logging config: `LOG_LEVEL`, `LOG_FORMAT`, `LOG_TO_CONSOLE`, `LOG_TO_FILE`, `LOG_FILE_PATH`, `LOG_MAX_SIZE_MB`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS`, `LOG_COMPRESS`.
- [x] Thêm monitoring config: `MONITORING_ENABLED`, `MONITORING_ADMIN_ROLES`, `METRICS_COLLECT_INTERVAL`, `PROMETHEUS_ENABLED`, `PROMETHEUS_PATH`, `MONGO_DEGRADED_THRESHOLD_MS`, `REDIS_DEGRADED_THRESHOLD_MS`.
- [x] Thêm rate limit config: `AUTH_RATE_LIMIT_ENABLED`, `AUTH_RATE_LIMIT_LOGIN_PER_MINUTE`, `AUTH_RATE_LIMIT_REFRESH_PER_MINUTE`, `AUTH_RATE_LIMIT_REGISTER_PER_MINUTE`, `RATE_LIMIT_FALLBACK`.
- [x] Thêm outbox config: `OUTBOX_ENABLED`, `OUTBOX_DRAIN_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_MAX_RETRIES`.
- [x] Thêm HTTP cache config: `ETAG_ENABLED`.
- [ ] Config có default an toàn cho local.
- [x] Production yêu cầu JWT current secret không rỗng.
- [x] JWT key format hỗ trợ `<key-id>/<base64-secret>` để tránh conflict ký tự trong secret.
- [x] JWT previous key có `NotAfter`; previous key chỉ dùng validate, không dùng ký token mới, và hết hiệu lực sau grace window cấu hình.
- [ ] Parse duration từ env.
- [x] Validate config lúc startup.
- [x] Production fail startup nếu `CORS_ALLOWED_ORIGINS` rỗng hoặc wildcard.
- [x] Local default chỉ allow `http://localhost:3000`, `http://localhost:5173`, `http://127.0.0.1:3000`, `http://127.0.0.1:5173`.
- [ ] Không log secret.

## 8. Bootstrap/runtime

Checklist:

- [ ] `cmd/api/main.go` load config.
- [ ] Init logger.
- [ ] Logger ghi ra terminal theo config.
- [ ] Logger ghi ra file theo config.
- [ ] Tạo request id generator.
- [ ] Init Prometheus registry/collectors.
- [x] Init outbox worker nếu enabled.
- [ ] Connect MongoDB.
- [ ] Connect Redis.
- [ ] Tạo cache adapter.
- [ ] Tạo mongo database adapter.
- [ ] Wrap bằng cached database.
- [ ] Chạy migration/bootstrap schema/index trước khi serve traffic theo policy.
- [ ] Ensure Mongo indexes.
- [ ] Log index created/exists/fail; index bắt buộc fail thì startup fail, index phụ trợ fail thì log error theo policy.
- [ ] Wire repositories.
- [ ] Wire services.
- [ ] Wire monitoring service.
- [ ] Wire admin guard middleware.
- [ ] Wire Prometheus middleware và `/metrics`.
- [ ] Wire handlers/router.
- [ ] Start HTTP server.
- [x] Graceful shutdown server, Mongo, Redis.
- [x] Stop outbox worker gracefully.
- [x] Flush/sync logger khi shutdown.
- [x] Health endpoint `/healthz` kiểm tra process sống.
- [x] Ready endpoint `/readyz` ping MongoDB và Redis với timeout.
- [x] Tạo `cmd/seed` CLI để seed admin user đầu tiên và test data local.
- [x] Tạo `cmd/migrate` CLI cho Mongo schema evolution/backfill/index changes.
- [x] Migration có version tracking collection, log version applied/skipped/failed.

## 9. Error handling và response

Checklist:

- [x] Tạo domain errors: `ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrConflict`, `ErrValidation`, `ErrTokenRevoked`, `ErrTokenExpired`.
- [x] Tạo `AppError` gồm `Code`, `Message`, `SafeMessage`, `HTTPStatus`, `Cause`, `Details`, `Op`, `Stack`, `Retryable`.
- [ ] `Op` ghi operation gây lỗi, ví dụ `AuthService.Login`, `SessionRepository.RotateRefreshToken`.
- [x] `Stack` chỉ populate ở dev/staging hoặc panic/5xx theo config; không trả stack ra client.
- [x] `Retryable` cho client/admin panel biết lỗi có nên retry không.
- [x] Tạo error code ổn định: `VALIDATION_ERROR`, `UNAUTHORIZED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `TOKEN_EXPIRED`, `TOKEN_REVOKED`, `INTERNAL_ERROR`, `DEPENDENCY_ERROR`.
- [x] Wrap lỗi ở layer thấp bằng cause, không mất lỗi gốc.
- [x] Service trả domain/app error, không trả lỗi driver thô trực tiếp lên handler.
- [x] Repository map Mongo duplicate key sang `ErrConflict`.
- [x] Repository map Mongo no documents sang `ErrNotFound`.
- [x] Cache/database map dependency timeout sang `DEPENDENCY_ERROR`.
- [x] Handler map domain error sang HTTP status.
- [x] Response lỗi thống nhất: `code`, `message`, `request_id`, `details` nếu là validation error an toàn.
- [x] Validation error details chuẩn hóa dạng array `{field, reason, meta}`; `reason` là code ổn định cho frontend i18n.
- [x] Response success thống nhất: `data`.
- [x] Không trả internal error detail ra client.
- [x] Middleware recovery bắt panic, log stack trace, trả `INTERNAL_ERROR`.
- [x] Middleware error handler log tất cả lỗi 5xx ở level error.
- [x] Middleware error handler log lỗi 4xx quan trọng ở level warn.
- [x] Log internal error với context: request id, user id, path, method, status, latency, error code, cause.
- [ ] Ghi error event phục vụ monitoring cho 5xx và các lỗi security quan trọng.
- [x] Unit test error mapper và response format.

## 10. Logging và tracing

Checklist:

- [ ] Mỗi request có `request_id`.
- [ ] Context keys dùng tập trung từ `internal/platform/ctxkeys`, không tự định nghĩa rời rạc.
- [ ] Nếu client gửi `X-Request-ID` hợp lệ thì reuse, nếu không server sinh mới.
- [ ] Response luôn trả `X-Request-ID`.
- [x] Nếu client/load balancer gửi `X-Trace-ID`, validate và đưa vào context/log; response có thể trả lại `X-Trace-ID`.
- [x] Log include `trace_id` và `span_id` nếu có trong context để sẵn sàng tích hợp OpenTelemetry sau này.
- [x] Access log ghi khi request kết thúc.
- [x] Access log có method, path, query, status, latency, ip, user agent, request id, user id nếu có.
- [ ] App log dùng structured JSON ở production.
- [ ] App log có console-friendly format ở local nếu config chọn.
- [x] Error log ghi stack/cause nội bộ cho panic và 5xx.
- [x] Auth service log event quan trọng: login success/failure, refresh success/failure, logout, token reuse suspected.
- [x] Database/cache wrapper log cache hit/miss, lock acquire timeout, Mongo/Redis dependency error ở level phù hợp.
- [ ] Không log request body mặc định.
- [ ] Nếu bật debug body logging, phải redact password/token/secret/email nếu config yêu cầu.
- [ ] Log file rotation được cấu hình.
- [x] Middleware inject logger vào context với `request_id`, và bổ sung `user_id/session_id` sau auth middleware.
- [x] Service/repository lấy logger bằng `logger.FromContext(ctx)` khi cần log.
- [ ] Test đảm bảo request id đi xuyên middleware -> service log -> response.

## 11. Monitoring service

Checklist:

- [ ] Tạo monitoring domain models: `SystemStatus`, `DependencyStatus`, `RuntimeMetrics`, `AuthStats`, `AuditLog`, `ErrorEvent`.
- [ ] `SystemStatus` gồm app name, version, env, uptime, started_at.
- [ ] `DependencyStatus` gồm MongoDB ping status, Redis ping status, latency, checked_at.
- [ ] Tạo `HealthLevel` enum: `healthy`, `degraded`, `unhealthy`.
- [ ] `DependencyCheck` gồm `status`, `latency_ms`, `error` safe message, `checked_at`.
- [ ] MongoDB/Redis status dùng degraded threshold từ config, timeout/error là unhealthy.
- [ ] `RuntimeMetrics` gồm goroutines, memory allocation, heap, GC count, process uptime.
- [ ] `AuthStats` gồm login success/failure count, active sessions, revoked sessions, refresh count, logout count.
- [x] Monitoring service dùng repository/query abstraction để lấy auth stats và audit logs.
- [x] Monitoring service có thể cache short TTL cho endpoint stats để tránh query nặng.
- [x] Dùng Prometheus client library chính thức cho metrics, không tự tạo metrics format từ đầu.
- [x] Expose `GET /metrics` theo Prometheus text exposition format.
- [x] HTTP middleware ghi metrics: request total, request duration, response status, method, route.
- [x] Cache/database layer ghi metrics: cache hit/miss, lock wait duration, Mongo/Redis dependency errors.
- [x] Auth service ghi metrics: login success/failure, refresh success/failure, logout, active sessions gauge nếu tính được.
- [ ] Monitoring endpoints trả response thống nhất như API còn lại.
- [ ] Monitoring endpoints không expose secret, connection string, token, stack trace raw cho non-admin.
- [ ] Chuẩn bị interface để sau này admin panel gọi trực tiếp qua HTTP API.

## 12. API policy: RBAC, pagination, rate limit

Checklist:

- [ ] Tạo middleware `AdminGuard` kiểm tra role `admin` từ user context/session.
- [ ] Tạo shared `Pagination` input gồm `limit`, `offset` hoặc cursor, `sort`, `from`, `to` khi cần filter thời gian.
- [ ] Cursor pagination mặc định dùng `_id`/`created_at` based cursor ổn định; không dùng skip/offset cho collection lớn nếu endpoint có thể tăng dữ liệu mạnh.
- [ ] Áp max limit mặc định cho list endpoint, ví dụ 100 records/request.
- [ ] Chuẩn hóa response list gồm `items`, `limit`, `offset`, `total` nếu query tính được.
- [ ] Tạo `internal/platform/ratelimit.Limiter` interface để có thể mock khi test.
- [ ] Implement Redis-backed rate limiter adapter.
- [ ] Áp rate limit cho `POST /v1/auth/login` theo IP + email.
- [ ] Áp rate limit cho `POST /v1/auth/register` theo IP.
- [ ] Áp rate limit cho `POST /v1/auth/refresh` theo IP + user/session nếu xác định được.
- [ ] Rate limit trả `429 TOO_MANY_REQUESTS` với error code ổn định.
- [x] Tạo ETag helper dùng stable hash của response payload cho `GET /v1/users/me` và `GET /v1/auth/devices`.
- [x] Khi `If-None-Match` match ETag hiện tại, trả `304 Not Modified` không body.

## 13. Security requirements

Checklist:

- [x] Password hash bằng bcrypt.
- [ ] Refresh token sinh bằng crypto random.
- [ ] Refresh token chỉ lưu hash.
- [ ] Access JWT có `sub`, `sid`, `jti`, `iat`, `exp`.
- [ ] JWT secret đọc từ env.
- [x] JWT header có `kid`; token service ký bằng current key và validate bằng current rồi fallback previous nếu configured.
- [ ] Refresh token rotation bắt buộc.
- [ ] Refresh token reuse sau rotation fail.
- [ ] Logout revoke session và blacklist access token.
- [ ] Logout ghi revoked access token `jti` vào Redis và Mongo `revoked_tokens` với TTL index.
- [x] Blacklist validation Redis miss thì fallback Mongo để chịu được Redis restart.
- [ ] Không log password/token.
- [ ] Validate input bằng validator.
- [x] CORS để cấu hình được, không hardcode wildcard cho production.
- [x] Production startup fail nếu CORS origin không explicit.
- [x] Request body size limit mặc định 1MB cho auth/admin endpoints, có thể cấu hình.
- [x] HTTP/server timeout và per-route timeout được cấu hình để tránh request treo vô hạn.
- [x] Refresh token binding: session lưu IP/UserAgent/DeviceID để audit; DeviceID không dùng làm security lookup.
- [x] Optional IP anomaly policy: nếu refresh từ IP khác bất thường, log warning/audit hoặc revoke theo config, mặc định chỉ audit để tránh false positive mobile network.
- [ ] Account lockout policy: sau N lần login fail trong T phút thì tạm khóa bằng Redis hoặc `LockedUntil`; nếu chưa implement captcha, ghi rõ trong README.
- [ ] Monitoring endpoints nằm sau auth và admin guard.
- [ ] Admin guard kiểm tra role `admin`; user thường không truy cập được `/v1/admin/*`.
- [ ] Rate limit login/register/refresh bằng Redis theo IP và email/user id.
- [ ] Logs và audit logs phải redact secret/token/password.

## 14. Testing checklist

### 14.1. Unit tests

- [ ] Cache Redis adapter: get/set/delete/cache miss.
- [ ] Cache lock: chỉ một caller giữ lock.
- [ ] Cached database read-through: miss -> Mongo -> set cache.
- [ ] Cached database cache hit: không gọi Mongo.
- [ ] Cached database stampede: concurrent miss chỉ gọi Mongo một lần.
- [ ] Cached database write: gọi Mongo rồi invalidate keys.
- [ ] User repository: create/find/update dùng database abstraction đúng options.
- [ ] Session repository: create/find/rotate/revoke/list active.
- [ ] Auth service register: hash password và tạo user.
- [ ] Auth service login success: tạo session, token, history.
- [ ] Auth service login failure: trả unauthorized và ghi history.
- [ ] Auth service refresh: rotate refresh token.
- [ ] Auth service refresh token reuse: fail.
- [ ] Auth service logout: revoke session và blacklist token.
- [ ] Token service: generate/validate/expired/blacklist.
- [ ] Logger writes console/file according to config.
- [ ] Logger redacts password/token/secret fields.
- [ ] Error mapper maps domain/app errors to correct HTTP status and code.
- [ ] Recovery middleware converts panic to `INTERNAL_ERROR`.
- [ ] Monitoring service returns system/runtime/dependency/auth stats from mocked dependencies.
- [ ] Admin guard allows role `admin` and rejects role `user`.
- [ ] Rate limiter allows under-limit requests and rejects over-limit requests.
- [ ] Refresh token rotation atomic update rejects stale old hash.
- [ ] Refresh token reuse revokes token family.
- [x] Prometheus collectors register without duplicate registration panic.
- [ ] Context keys không conflict giữa middleware/logger/service.
- [x] Database read/write options validate `ReadOptions`/`WriteOptions` và lock fallback đúng policy.
- [ ] `FindMany` không cache nếu không có explicit `CacheKey`.
- [ ] `FindMany` chỉ cache với explicit normalized `CacheKey` và `CacheableFilter`.
- [ ] Session rotation invalidates `session:id`, old `session:refresh`, and active device list keys.
- [ ] Outbox enqueue/drain/retry handles temporary audit write failure.
- [ ] Outbox unique `IdempotencyKey` ngăn duplicate khi caller retry sau timeout.
- [x] DeviceID validation rejects invalid/oversized input and never drives security lookup.
- [x] JWT `kid` validates current and previous key.
- [x] JWT previous key hết hiệu lực theo `NotAfter`.
- [x] Redis blacklist miss falls back to Mongo revoked token repository.
- [x] HealthLevel maps dependency latency/error to healthy/degraded/unhealthy.
- [x] Rate limit fallback `block/allow` hoạt động đúng khi Redis down.
- [x] Validation error details use `{field, reason, meta}` schema.

### 14.2. Handler tests

- [ ] Register success.
- [ ] Register duplicate email.
- [ ] Login success.
- [ ] Login wrong password.
- [ ] Refresh success.
- [ ] Refresh with old token fails.
- [ ] Logout success.
- [ ] Protected endpoint without token returns 401.
- [ ] Protected endpoint with blacklisted token returns 401.
- [ ] Devices endpoint returns active devices.
- [x] Login history endpoint paginates.
- [x] Error response always includes `request_id`.
- [x] Response header always includes `X-Request-ID`.
- [x] Monitoring endpoints require auth/admin guard.
- [x] Monitoring status/dependencies/runtime endpoints return expected shape.
- [x] User role cannot access `/v1/admin/*`.
- [x] Admin role can access `/v1/admin/*`.
- [x] Auth rate limit returns 429 when exceeded.
- [x] `/metrics` returns Prometheus exposition format when enabled.
- [x] `GET /v1/users/me` and device list return 304 for matching ETag.
- [x] Validation errors return field-level details with stable reason codes.

### 14.3. Integration tests

- [x] Docker Compose starts MongoDB and Redis.
- [x] API boots, `/healthz` returns success, and `/readyz` returns success when dependencies are ready.
- [x] End-to-end register -> login -> me -> refresh -> logout -> me fails.
- [x] Logout all invalidates multiple sessions.
- [x] Cache key invalidates after user/session update.
- [x] Request writes access log to terminal/file path.
- [x] 5xx test path writes error log and error event with request id.
- [x] Monitoring auth stats reflect login/logout activity.
- [x] Prometheus metrics include HTTP request counters/duration after traffic.
- [x] Transaction-capable environment handles register/login multi-write consistently.
- [x] Non-transaction local environment logs audit/history failure, enqueues outbox event, and does not fail auth.
- [x] Redis restart simulation still rejects Mongo-persisted revoked access token.
- [x] `cmd/seed` can create the first admin idempotently.
- [x] `cmd/migrate` applies a migration once and records version.

## 15. Documentation checklist

- [x] Tạo `README.md` hướng dẫn chạy local.
- [x] Tạo `.env.example`.
- [x] Tạo `docker-compose.yml` cho MongoDB + Redis.
- [ ] Ghi danh sách endpoint và request/response mẫu.
- [ ] Ghi kiến trúc layer và dependency direction.
- [ ] Ghi cách mock database/cache/repository/service khi test.
- [ ] Ghi policy refresh token rotation và logout invalidation.
- [ ] Ghi logging config, log format, log file path, rotation policy.
- [x] Ghi error response format và error code table.
- [x] Ghi monitoring endpoints phục vụ admin panel.
- [x] Ghi RBAC/admin role policy.
- [x] Ghi pagination/filter convention.
- [x] Ghi auth rate limit policy.
- [x] Ghi Prometheus `/metrics` setup và ví dụ scrape config.
- [x] Ghi Mongo transaction requirement nếu muốn atomic multi-document writes.
- [x] Ghi quy ước `FindMany` cache explicit-only và trách nhiệm `InvalidateKeys`.
- [x] Ghi DeviceID trust model.
- [x] Ghi JWT `kid` rotation process.
- [x] Ghi outbox behavior và at-least-once semantics.
- [x] Ghi revoked token Redis + Mongo fallback.
- [x] Ghi `HealthLevel` threshold config.
- [x] Ghi ETag behavior cho poll-heavy endpoints.
- [x] Ghi validation error schema.
- [x] Ghi graceful degradation matrix cho Redis/Mongo/outbox/rate limit.
- [x] Ghi context propagation contract và danh sách key chuẩn.
- [x] Ghi seed admin user bằng `cmd/seed`.
- [x] Ghi migration/backfill/index strategy bằng `cmd/migrate`.
- [x] Ghi JWT key format `<key-id>/<base64-secret>` và previous key `NotAfter`.
- [x] Ghi refresh token binding là audit/anomaly signal, không phải security lookup chính.
- [x] Ghi account lockout policy hoặc lý do intentionally scoped out.

## 16. Thứ tự implement đề xuất

- [ ] Phase 1: Project bootstrap, config, logger, Docker Compose, context keys, seed/migrate skeleton.
- [ ] Phase 2: Error model, response format, recovery/error middleware, request id, logging middleware, CORS/body-limit/timeout middleware.
- [ ] Phase 3: Cache interface + Redis implementation + lock/rate-limit primitives + tests.
- [ ] Phase 4: Database interface + Mongo adapter + typed `ReadOptions`/`WriteOptions` + cached database wrapper + degradation tests.
- [ ] Phase 5: Domain entities + repository interfaces + cache key helpers + pagination/cursor primitives.
- [ ] Phase 6: Mongo repository implementations + index bootstrap + migration/seed commands.
- [ ] Phase 7: Token service only: JWT `kid`, current/previous key, `NotAfter`, refresh token crypto/hash, blacklist abstraction.
- [ ] Phase 8: Auth service skeleton + Gin router + auth endpoints với thin handlers để có endpoint test thủ công sớm.
- [ ] Phase 9: Hoàn thiện auth: rate limit, ETag, refresh rotation, token family reuse handling, outbox, revoked-token Mongo fallback.
- [ ] Phase 10: RBAC/admin guard + user/device/login-history endpoints + account lockout/anomaly audit policy.
- [ ] Phase 11: Monitoring service + audit/error event repository + Prometheus collectors + admin monitoring endpoints + `/metrics`.
- [ ] Phase 12: Integration tests and README.
- [ ] Phase 13: Hardening pass: security, errors, logging, graceful shutdown, lint, race/vuln checks, Docker hardening.


### 16.1. Hardening pass chi tiết

Checklist:

- [x] `govulncheck ./...` pass.
- [x] `golangci-lint run` pass với config strictness phù hợp cho template.
- [x] `go test -race ./...` pass.
- [x] Test goroutine leak bằng `goleak` cho service/worker quan trọng.
- [ ] Tất cả exported type/function quan trọng có godoc.
- [ ] Dependency được pin version trong `go.sum`.
- [x] Docker image build bằng non-root user.
- [ ] Secret không được bake vào Docker image hoặc log startup.
- [x] Graceful shutdown dừng HTTP server, Mongo/Redis client, Prometheus/metrics routines, outbox worker, logger flush.
- [x] Request body size limit và response/per-route timeout được test.
- [x] CORS production không cho wildcard và startup fail nếu config thiếu.
- [x] Mongo pool/read preference config được validate.
- [x] Redis TLS config được document và parse đúng.

## 17. Acceptance criteria

- [ ] `go test ./...` pass.
- [ ] API chạy được bằng config local.
- [ ] MongoDB và Redis chạy được qua Docker Compose.
- [ ] Register/login/refresh/logout hoạt động end-to-end.
- [ ] Access token logout xong không dùng lại được.
- [ ] Refresh token cũ sau rotation không dùng lại được.
- [ ] User xem được danh sách thiết bị đang đăng nhập.
- [ ] User xem được lịch sử đăng nhập.
- [ ] Repository không phụ thuộc Redis hoặc Mongo client cụ thể.
- [ ] Service không phụ thuộc Gin hoặc database driver.
- [ ] Handler không chứa business logic.
- [ ] Database/cache có thể mock trong unit test.
- [ ] Cache coordination nằm dưới database abstraction, không nằm trong repository.
- [x] Log được ghi ra terminal.
- [x] Log được ghi ra file theo config.
- [ ] Mọi response lỗi có error code và request id.
- [ ] Panic được recovery, log stack/cause, và trả response an toàn.
- [x] Monitoring endpoints cung cấp system status, dependency status, runtime metrics, auth stats, recent errors, audit logs.
- [x] Monitoring implementation đi qua service/repository/interface, không phụ thuộc trực tiếp driver cụ thể ở handler.
- [x] `/v1/admin/*` chỉ admin role truy cập được.
- [x] Auth endpoints có Redis-backed rate limit.
- [x] Refresh token rotation là atomic và detect reuse.
- [x] Reuse refresh token cũ revoke toàn bộ token family theo policy.
- [x] `/metrics` expose Prometheus metrics khi bật config.
- [x] Pagination/filter thống nhất cho login history, audit logs, recent errors.
- [x] Mongo-specific query types không leak ra domain/service/handler.
- [x] `FindMany` cache chỉ hoạt động với explicit normalized `CacheKey` và `CacheableFilter`.
- [x] Session rotation invalidates all related old session/refresh/device-list cache keys.
- [x] Audit/error/history events không mất vĩnh viễn khi write tạm fail; outbox retry at-least-once.
- [ ] DeviceID chỉ là UX hint, được validate, và không dùng làm security lookup.
- [ ] JWT `kid` rotation hỗ trợ current/previous key và previous key `NotAfter`.
- [x] Logout security chịu được Redis restart nhờ Mongo `revoked_tokens` fallback.
- [x] Dependency health trả `healthy/degraded/unhealthy` theo threshold config.
- [x] `GET /v1/users/me` và device list hỗ trợ ETag/304.
- [x] Validation errors có field-level stable reason codes.
- [x] Database abstraction dùng typed read/write options, lock fallback/strict behavior rõ ràng và test được.
- [x] `FindMany` cache được enforce bằng `CacheableFilter`, không chỉ là convention.
- [x] Outbox có unique `IdempotencyKey` và worker idempotent để tránh duplicate audit/history.
- [x] Token family reuse detection phân biệt active-stale hash, logout, expired session và race theo policy.
- [x] `cmd/seed` seed được admin đầu tiên mà không qua public register.
- [x] `cmd/migrate` quản lý index/backfill/schema evolution có version tracking.
- [x] Graceful degradation matrix được implement/config rõ cho Redis/Mongo/outbox/rate limit.
- [x] `/healthz` và `/readyz` có behavior khác nhau, timeout rõ ràng.
- [x] CORS, body size limit, HTTP timeout, Mongo pool, Redis TLS đều có config và validation.
