# Risk Engine Project Architecture

Version: `2026-05-03`

This document gives maintainers a current map of the repository: what each
service does, where code lives, how requests flow, and which architectural rules
to preserve when adding features.

## Repository Overview

This repository contains two Go services:

| Service | Entry point | Main responsibility |
| --- | --- | --- |
| Risk service | `cmd/risk-server/main.go` | Risk-control decisions, rate limits, blacklist checks, risk insight/admin APIs. |
| Captcha service | `cmd/captcha-server/main.go` | Slider captcha generation/verification, short-lived token issuance, runtime image-source management. |

Shared infrastructure lives under `internal/shared`:

- `internal/shared/config`: config loading, env overrides, strict validation
- `internal/shared/logging`: zap logger setup
- `internal/shared/registry`: Nacos registration
- `internal/shared/health`: Redis-backed health checks

Runtime configuration files live under `configs`:

- `risk.dev.yaml`, `risk.prod.yaml`, `risk.template.yaml`
- `captcha.dev.yaml`, `captcha.prod.yaml`, `captcha.template.yaml`

Swagger artifacts and project docs live under `docs`.

## Top-Level Layout

```text
cmd/
  risk-server/          risk service composition root
  captcha-server/       captcha service composition root

internal/
  risk/                 current risk service implementation
  captcha/              captcha service, refactored around hexagonal architecture
  shared/               config, logging, health, registry helpers

configs/                service runtime config files
docs/                   architecture, API, Swagger, refactor docs
web-test/               small manual/test clients
```

## Runtime Dependencies

Both services depend on:

- Redis for runtime state
- Nacos for optional service registration
- zap for logging
- gRPC for internal RPC APIs
- Gin for HTTP APIs

The captcha service also depends on:

- `go-captcha` for slider captcha generation
- an optional external image API for background image pool refresh

The protobuf-generated service definitions come from:

`github.com/Pupervemon/risk-proto`

## Composition Roots

The `cmd/*/main.go` files are composition roots. They are allowed to know about
configuration, Redis, HTTP/gRPC servers, Nacos, logging, and concrete adapters.

Application/domain packages should not perform this wiring.

Common startup pattern:

1. Parse command-line config flags.
2. Load service config from `configs`.
3. Create logger.
4. Create Redis client and ping it.
5. Construct service/application components.
6. Create HTTP and gRPC servers.
7. Register with Nacos.
8. Wait for shutdown signal.
9. Deregister and gracefully stop servers.

## Captcha Architecture

The captcha service has been refactored into a hexagonal structure. The old
`internal/captcha/service` package has been removed.

Current directories:

```text
internal/captcha/
  domain/               captcha and image-source domain models/errors
  application/          use cases and application services
  application/ports/    inbound and outbound port interfaces
  adapter/inbound/      HTTP and gRPC adapters
  adapter/outbound/     Redis, captcha generator, image, image-pool adapters
  bootstrap/            production wiring helpers used by cmd/captcha-server
```

### Captcha Dependency Direction

Keep this direction:

```text
cmd/captcha-server
  -> bootstrap
  -> application ports and concrete adapters

adapter/inbound
  -> application/ports
  -> domain

application
  -> application/ports
  -> domain

adapter/outbound
  -> application/ports
  -> domain
```

The following dependencies must not leak into `internal/captcha/domain` or
`internal/captcha/application`:

- Gin
- gRPC transport types
- Redis client
- `go-captcha`
- `internal/shared/config`
- `net/http`
- old `internal/captcha/service`

### Captcha Domain

`internal/captcha/domain` contains plain models and domain-level errors:

- slider captcha challenge/answer models
- mouse-track points
- image pool metadata
- runtime image-source config/patch/status
- image-source refresh and persistence error wrappers

Domain code should stay free of infrastructure and transport concerns.

### Captcha Application

`internal/captcha/application` contains use cases:

- `CaptchaUseCase`: generate and verify slider captchas
- `TokenUseCase`: issue and verify short-lived captcha tokens
- `ImageSourceUseCase`: status, validate, update, refresh runtime image source
- `CaptchaLifecycle`: start/stop image-pool refresh lifecycle
- `RuntimeImageSourceManager`: in-memory runtime image-source config/provider owner
- `track_validator.go`: mouse-track validation logic

Application code talks to persistence, image generation, image pools, and
external providers through ports in `internal/captcha/application/ports`.

### Captcha Inbound Adapters

HTTP adapter:

`internal/captcha/adapter/inbound/http`

Responsibilities:

- parse HTTP JSON payloads
- call application use case ports
- map domain/application results to HTTP responses
- perform admin-header authorization for admin endpoints
- expose Swagger annotations/models

gRPC adapter:

`internal/captcha/adapter/inbound/grpc`

Responsibilities:

- implement protobuf gRPC service methods
- call application token use case
- map application results to proto responses

Inbound adapters should depend on application ports, not concrete application
structs unless there is a deliberate reason.

### Captcha Outbound Adapters

Redis adapters:

`internal/captcha/adapter/outbound/redis`

Responsibilities:

- captcha answer repository
- token repository
- image-pool image metadata/data repository
- runtime image-source config store

Captcha generator adapter:

`internal/captcha/adapter/outbound/captcha`

Responsibilities:

- wrap `go-captcha`
- generate slider captcha images and answers

External image adapter:

`internal/captcha/adapter/outbound/image`

Responsibilities:

- build runtime image providers
- call upstream image API
- parse direct image, JSON, URL, base64, and data URI responses
- normalize images to PNG and target dimensions

Image-pool adapter:

`internal/captcha/adapter/outbound/imagepool`

Responsibilities:

- maintain Redis-backed background image pool
- perform refresh scheduling and locking
- expose image-pool behavior through application ports

### Captcha Bootstrap

`internal/captcha/bootstrap` is where production captcha components are assembled:

- `NewCaptchaComponents`
- `NewRuntimeImageSourceComponents`
- `NewTokenUseCase`
- config-to-application/adapter option mapping
- image-pool construction
- persisted runtime image-source restore

Bootstrap may depend on shared config and concrete adapters. Application and
domain packages should not.

### Captcha Request Flow

Generate captcha:

```text
HTTP GET /api/v1/captcha
  -> adapter/inbound/http.CaptchaHandler
  -> application.CaptchaUseCase.Generate
  -> outbound captcha generator
  -> optional image pool
  -> Redis answer repository
```

Verify captcha and issue token:

```text
HTTP POST /api/v1/captcha/verify
  -> adapter/inbound/http.CaptchaHandler
  -> application.CaptchaUseCase.Verify
  -> Redis answer repository
  -> application.TokenUseCase.Issue
  -> Redis token repository
```

Verify token over gRPC:

```text
gRPC CaptchaTokenService.VerifyToken
  -> adapter/inbound/grpc.CaptchaTokenService
  -> application.TokenUseCase.Verify
  -> Redis token repository
```

Runtime image-source update:

```text
HTTP PUT /api/v1/admin/image-source
  -> adapter/inbound/http.ImageSourceAdminHandler
  -> application.ImageSourceUseCase.Update
  -> application.RuntimeImageSourceManager
  -> outbound image provider factory/fetcher
  -> Redis runtime image-source store
  -> image-pool refresh
```

## Risk Service Architecture

The risk service currently uses a simpler layered structure:

```text
internal/risk/
  service/              risk business logic and gRPC server implementation
  transport/            HTTP health/info/admin routes and auth
```

`cmd/risk-server/main.go` wires:

- config
- logger
- Redis
- `riskservice.NewRiskService`
- risk HTTP router
- risk gRPC server
- Nacos registration

### Risk Service Responsibilities

`internal/risk/service` owns:

- gRPC `RiskControlService` implementation
- IP/user blacklist checks
- IP rate limiting
- user scoped rate limiting
- login failure counters
- event reporting
- risk insight recording

`internal/risk/transport` owns:

- HTTP health endpoint
- service info endpoint
- admin risk IP summary/event endpoints
- admin authorization middleware
- HTTP Swagger models/docs

Risk has not yet been refactored into the same hexagonal shape as captcha. Do
not assume captcha and risk have identical layering.

## Shared Config

Config loaders are in `internal/shared/config`.

Use:

- `LoadRiskConfigWithOptions`
- `LoadCaptchaConfigWithOptions`

Both services read from the `configs` directory by default and allow CLI options:

- `-config`
- `-env`

Config shape is shared infrastructure. Keep direct `internal/shared/config`
usage in composition roots and bootstrap/wiring layers. Avoid pulling config
types into domain/application business logic.

## Service Registration And Health

Nacos registration is handled by:

`internal/shared/registry/nacos.go`

Health helpers are under:

`internal/shared/health`

Both services connect to Redis at startup and fail fast if Redis is unavailable.

HTTP health endpoints are provided by service-specific routers/handlers:

- captcha: `internal/captcha/adapter/inbound/http/health_handler.go`
- risk: `internal/risk/transport/health_handler.go`

## Documentation Map

Useful documents:

- `docs/PROJECT_ARCHITECTURE.md`: this file
- `docs/CAPTCHA_HEXAGONAL_REFACTOR_PLAN.md`: original captcha refactor plan
- `docs/CAPTCHA_HEXAGONAL_REFACTOR_PROGRESS.md`: current captcha refactor status
- `docs/CAPTCHA_RUNTIME_IMAGE_SOURCE_GUIDE.md`: current runtime image-source path
- `docs/CAPTCHA_SERVICE_API.md`: captcha API summary
- `docs/RISK_SERVICE_API.md`: risk API summary
- `docs/CONFIG_GUIDE.md`: config and environment guidance

Swagger artifacts:

- `docs/swagger/captcha`
- `docs/swagger/risk`

## Development Rules

When modifying captcha:

1. Keep domain free of infrastructure.
2. Keep application free of Redis/Gin/gRPC/shared config.
3. Add new external dependencies behind outbound ports.
4. Add new HTTP/gRPC behavior in inbound adapters.
5. Put production wiring in `bootstrap` or `cmd/captcha-server`.
6. Do not recreate `internal/captcha/service`.

When modifying risk:

1. Be aware it is still more directly layered than captcha.
2. Keep HTTP concerns in `internal/risk/transport`.
3. Keep Redis key behavior clear and documented near service logic.
4. Consider captcha's hexagonal layout as the likely direction for future risk refactors.

## Validation Commands

For captcha-focused changes:

```bash
go test ./internal/captcha/... ./internal/shared/config/...
go build ./cmd/captcha-server
```

For risk-focused changes:

```bash
go test ./internal/risk/... ./internal/shared/config/...
go build ./cmd/risk-server
```

For broader changes:

```bash
go test ./...
```

Captcha boundary scan:

```bash
rg "internal/captcha/service|github.com/redis/go-redis|github.com/gin-gonic|google.golang.org/grpc|github.com/wenlng/go-captcha|internal/shared/config|net/http" internal/captcha/application internal/captcha/domain -n
```

The scan should have no matches.
