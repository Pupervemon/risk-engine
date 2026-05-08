# Captcha Runtime Image Source Guide

Version: `2026-05-03`

Status: `current after hexagonal refactor`

This guide explains the runtime image-source hot-update path in `captcha-server`
after the hexagonal refactor. The old `internal/captcha/service` package has
been removed. Runtime image-source behavior now flows through:

- inbound adapter: `internal/captcha/adapter/inbound/http`
- application use case and manager: `internal/captcha/application`
- outbound ports: `internal/captcha/application/ports`
- outbound adapters: `internal/captcha/adapter/outbound/*`
- bootstrap wiring: `internal/captcha/bootstrap`
- composition root: `cmd/captcha-server`

## Current Mental Model

There are four separate concepts that should not be collapsed into one:

1. Startup config: `config.CaptchaConfigSpec.ExternalImageAPI`
2. Runtime effective config: `domain.ImageSourceRuntimeConfig`
3. Persisted runtime config: Redis key `captcha:image-source:runtime-config`
4. Active image provider: the provider currently used by the background image pool

Startup config is only the initial default. Runtime updates validate and apply a
new provider while the process is running. Successful updates are persisted to
Redis so the next restart can restore the last accepted runtime config.

## Layer Map

| Layer | Current code | Responsibility |
| --- | --- | --- |
| Composition root | `cmd/captcha-server/main.go` | Loads config, creates Redis/logger, calls bootstrap, wires HTTP/gRPC. |
| Bootstrap | `internal/captcha/bootstrap/runtime_image_source.go` | Builds the runtime manager, Redis store, provider factory, use case, and restores persisted config. |
| Inbound HTTP adapter | `internal/captcha/adapter/inbound/http/image_source_handler.go` | Parses admin requests, calls `ImageSourceUseCase`, maps domain errors to HTTP responses. |
| Application use case | `internal/captcha/application/image_source_usecase.go` | Coordinates validate/update/refresh/status operations. |
| Runtime manager | `internal/captcha/application/runtime_image_source_manager.go` | Holds active config/provider, validates candidates, records status. |
| Domain model | `internal/captcha/domain/image_source.go` | Defines config, patch, status, validation result, pool snapshot. |
| Domain errors | `internal/captcha/domain/errors.go` | Defines disabled/refresh/persistence error semantics. |
| Outbound ports | `internal/captcha/application/ports/outbound.go` | Defines image provider, provider factory, image pool, runtime store, runtime manager interfaces. |
| Redis store adapter | `internal/captcha/adapter/outbound/redis/image_source_store.go` | Persists and loads runtime image-source config from Redis. |
| External image adapter | `internal/captcha/adapter/outbound/image` | Builds providers and fetches/normalizes upstream images. |
| Image-pool adapter | `internal/captcha/adapter/outbound/imagepool` | Exposes Redis-backed image pool through application ports. |

## Startup Flow

`cmd/captcha-server/main.go` wires the current runtime image-source path like this:

1. Load `config.CaptchaConfig`.
2. Build Redis and logger.
3. Call `bootstrap.NewCaptchaComponents`.
4. Call `bootstrap.NewRuntimeImageSourceComponents`.
5. Pass `imageSourceComponents.UseCase` into `http.ImageSourceAdminHandler`.
6. Start the image-pool lifecycle if `cfg.Captcha.ImagePool.Enabled` is true.

`bootstrap.NewRuntimeImageSourceComponents` does the runtime-image-source setup:

1. If the image pool is nil, return a disabled `ImageSourceUseCase`.
2. Normalize captcha image dimensions, falling back to `320x180`.
3. Convert shared config into `domain.ImageSourceRuntimeConfig`.
4. Build an `adapter/outbound/image.ExternalImageProviderFactory`.
5. Create `application.RuntimeImageSourceManager`.
6. Create `adapter/outbound/redis.ImageSourceStore`.
7. Try to restore persisted runtime config from Redis with a 3 second timeout.
8. Set the image pool provider to the runtime manager.
9. Return `RuntimeImageSourceComponents{Manager, UseCase}`.

If persisted Redis config is invalid or cannot build a provider, bootstrap keeps
the file-based startup config and logs a warning.

## Runtime Manager

`application.RuntimeImageSourceManager` is the in-memory owner of runtime state.
It implements two important port roles:

- `RuntimeImageSourceManager`
- `ImageProvider`

As a manager, it can:

- merge a partial `domain.ImageSourcePatch` into the current config
- validate basic config shape
- build and validate a candidate provider
- apply or restore config/provider state
- record validation and refresh outcomes
- return a sanitized `domain.ImageSourceStatus`

As an image provider, it delegates `FetchImages(ctx, count)` to the currently
active provider. This is why the image pool can keep using one provider reference
while runtime updates switch the underlying upstream source.

The manager protects mutable state with a mutex. It reads the active provider
under lock, then performs upstream fetching outside the lock.

## Use Case Behavior

`application.ImageSourceUseCase` exposes the inbound port used by HTTP:

- `Status(ctx)`
- `Validate(ctx, patch)`
- `Update(ctx, patch, triggerRefresh)`
- `Refresh(ctx)`

### Status

`Status` reads the image-pool snapshot and combines it with manager state:

- current sanitized config
- config version
- update time
- last validation time/error
- last refresh time/error
- configured pool size
- current image count
- active generation metadata

If runtime image-source is not available, it returns `Enabled: false`.

### Validate

`Validate` does not apply or persist anything.

Flow:

1. Build candidate config from patch.
2. Validate basic fields.
3. Build a provider from candidate config.
4. Fetch one image to prove upstream availability.
5. Record validation result in the manager.
6. Return sanitized validation result.

### Update

`Update` validates, persists, applies, and optionally refreshes.

Flow:

1. Build candidate config from patch.
2. Build and validate a provider.
3. Save candidate config to Redis if a store exists.
4. Apply config/provider to the manager.
5. If `triggerRefresh` is true, refresh the image pool with the new provider.
6. Record refresh result.
7. Return current status.

Error semantics:

- Config validation/build failure returns the original error.
- Redis save failure returns `domain.ImageSourcePersistenceError`.
- Refresh failure returns status plus `domain.ImageSourceRefreshError`.
- A disabled image pool returns `domain.ErrImagePoolDisabled`.

Current behavior intentionally persists and applies the new provider before the
optional refresh result is known. If refresh fails, status is returned with the
refresh error recorded.

### Refresh

`Refresh` uses the currently active provider and refreshes the image pool.

Flow:

1. Call `pool.Refresh(ctx)`.
2. Record refresh result in the manager.
3. Return status.

Refresh errors are wrapped as `domain.ImageSourceRefreshError`.

## HTTP Admin API

The HTTP admin handler is in:

`internal/captcha/adapter/inbound/http/image_source_handler.go`

Routes are mounted under `/api/v1/admin`:

| Method | Path | Use case method |
| --- | --- | --- |
| `GET` | `/image-source` | `Status` |
| `POST` | `/image-source/validate` | `Validate` |
| `PUT` | `/image-source` | `Update` |
| `POST` | `/image-source/refresh` | `Refresh` |

Request patch fields:

- `url`
- `apiKey`
- `timeoutSeconds`
- `rateLimitPerMinute`
- `retryCount`
- `triggerRefresh` for update only

The handler maps application/domain errors to HTTP responses:

- `ErrImagePoolDisabled` -> `409 IMAGE_POOL_DISABLED`
- `ErrImagePoolRefreshInProgress` -> `409 IMAGE_POOL_REFRESH_IN_PROGRESS`
- `ImageSourceRefreshError` -> `502`
- `ImageSourcePersistenceError` -> `500`
- validation/rejection errors -> `400`

The API response never returns the API key value. It only returns
`apiKeyConfigured`.

## Redis Persistence

The Redis adapter is:

`internal/captcha/adapter/outbound/redis/image_source_store.go`

Key:

`captcha:image-source:runtime-config`

Stored fields:

- `url`
- `apiKey`
- `timeoutSeconds`
- `rateLimitPerMinute`
- `retryCount`
- `updatedAt`

`Load` returns `(config, found, error)`. Missing key is not an error.

`Save` stores the payload without expiration.

## External Image Fetching

The provider factory is:

`internal/captcha/adapter/outbound/image/provider_factory.go`

It converts `domain.ImageSourceRuntimeConfig` into the adapter config and builds
an `ExternalImageFetcher`.

`ExternalImageFetcher` supports:

- direct image responses
- JSON responses containing image URL fields
- JSON responses containing inline image/base64 fields
- nested payloads under common keys such as `data`, `result`, `payload`, `body`
- relative image URLs resolved against the API URL
- data URI and raw base64 images
- response size limits
- retry with simple backoff
- rate limiting
- image normalization to PNG
- resize/crop to target captcha dimensions

If the image-pool provider is built from static config and `ExternalImageAPI.URL`
is empty, the adapter can fall back to `MockImageFetcher`. Runtime manager
creation, however, validates its initial runtime config and requires a valid URL.

## Image Pool Interaction

The image pool adapter is:

`internal/captcha/adapter/outbound/imagepool/port_adapter.go`

The application sees the pool through:

- `BackgroundImagePool`
- `ManagedBackgroundImagePool`

Runtime image-source uses pool capabilities to:

- read snapshots for status
- refresh with current provider
- refresh with a newly validated provider
- set the runtime manager as the active provider during bootstrap
- report pool size

Generation use cases only need the smaller background-image pool behavior. Runtime
management uses the managed port because it needs provider switching and pool-size
status.

## Status Fields

The status returned by admin APIs is derived from `domain.ImageSourceStatus`:

- `enabled`: runtime management available
- `version`: increments when a new config is applied
- `config`: sanitized current config
- `updatedAt`: last applied/restored config time
- `lastValidatedAt`
- `lastValidationError`
- `lastRefreshedAt`
- `lastRefreshError`
- `poolSize`
- `poolImageCount`
- `activeGeneration`
- `generationCount`

`updatedAt`, validation fields, and refresh fields are formatted as RFC3339
strings when present.

## Important Current Behaviors

1. The old `internal/captcha/service` package no longer exists.
2. `cmd/captcha-server` uses `bootstrap` directly.
3. HTTP and gRPC are inbound adapters under `internal/captcha/adapter/inbound`.
4. Runtime state lives in `application.RuntimeImageSourceManager`.
5. Redis persistence is an outbound adapter, not application logic.
6. External image fetching is an outbound adapter, not application logic.
7. Runtime updates are local to the current process unless other instances also
   load or apply the Redis config.
8. The current Redis payload does not include operator identity, version, or
   source metadata beyond `updatedAt`.

## Operational Notes

When debugging runtime image-source behavior, check in this order:

1. `cmd/captcha-server/main.go` for wiring.
2. `internal/captcha/bootstrap/runtime_image_source.go` for startup/restore.
3. `internal/captcha/application/image_source_usecase.go` for operation flow.
4. `internal/captcha/application/runtime_image_source_manager.go` for state.
5. `internal/captcha/adapter/inbound/http/image_source_handler.go` for API mapping.
6. `internal/captcha/adapter/outbound/redis/image_source_store.go` for persisted config.
7. `internal/captcha/adapter/outbound/image/fetcher.go` for upstream fetch issues.
8. `internal/captcha/adapter/outbound/imagepool` for refresh and pool state.

Common failure areas:

- image pool disabled
- invalid or empty runtime URL
- upstream API returns JSON without recognizable image fields
- upstream image endpoint returns text/JSON instead of an image
- API key missing or invalid
- Redis write failure during update
- image-pool refresh already in progress
- upstream fetch succeeds during validation but fails during pool refresh

## Future Improvements

These are not part of the current refactor slice, but remain useful follow-ups:

1. Add reset-to-file-config and delete-persisted-runtime-config admin operations.
2. Add Redis Pub/Sub or polling so runtime changes propagate across instances.
3. Add operator metadata to the Redis payload.
4. Add an explicit state model for validated, persisted, applied, and refreshed.
5. Add focused integration coverage for the HTTP admin update path.
