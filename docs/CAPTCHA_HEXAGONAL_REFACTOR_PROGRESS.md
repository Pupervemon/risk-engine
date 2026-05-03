# Captcha Hexagonal Refactor Progress

This document tracks the current implementation status against
`docs/CAPTCHA_HEXAGONAL_REFACTOR_PLAN.md`.

## Current Status

Completed in the current refactor slices:

1. `domain` and `application/ports` are in place for captcha, token, image pool, and runtime image-source boundaries.
2. Redis answer storage, token storage, image-pool storage, runtime image-source store, go-captcha generation, external image fetching, and image-pool runtime behavior are represented as outbound adapters.
3. `RuntimeImageSourceManager` lives in `application` and depends on `ImageProviderFactory` as an outbound application port.
4. `ExternalImageProviderFactory` lives in `adapter/outbound/image`.
5. Runtime image-source behavior is delegated to application use cases.
6. Captcha, image-pool, runtime image-source, and token production wiring has moved into `bootstrap`.
7. Shared-config to application/adapter option mapping has moved into `bootstrap/config_mapper.go`.
8. The old duplicate `service` track validator has been removed; track validation behavior now lives in `application`.
9. The compatibility-only `service` image-pool builder wrapper has been removed; its coverage now targets `bootstrap.NewConfiguredImagePool` directly.
10. Runtime image-source restoration now goes through the `RuntimeImageSourceManager` port instead of depending on the concrete application manager type from `service`.
11. Token test wiring now goes through `bootstrap.NewTokenUseCaseWithRepository`; `service/token.go` no longer imports the concrete application package.
12. Obsolete `service` image-provider aliases were removed.
13. The unused legacy use-case adapter bridge was removed; `cmd/captcha-server` now gets application ports directly from the thin service facade.
14. `CaptchaService` now holds image-pool behavior through the application image-pool port instead of the concrete Redis image-pool adapter.
15. Runtime image-source persisted-config restoration moved from `service` into `bootstrap`.
16. Image-pool runtime management is represented by `ManagedBackgroundImagePool`, keeping the basic `BackgroundImagePool` port smaller for generation/lifecycle use cases.
17. `service` no longer stores runtime image-source manager/store binding state; it keeps only the enabled image-source use case.
18. `cmd/captcha-server` now uses `bootstrap` components directly; production code no longer imports `internal/captcha/service`.
19. Legacy `service` image-source errors, constructors, DTO mappings, and facade methods were removed after confirming no repo-local caller depends on them.
20. `internal/captcha/service` has been deleted; captcha behavior now flows through `application` ports, inbound adapters, outbound adapters, and `bootstrap`.
21. HTTP inbound adapter files moved from `internal/captcha/transport/http` to `internal/captcha/adapter/inbound/http`; `cmd/captcha-server` now imports the new path.

## Validation

The following commands passed after the latest slices:

```bash
go test ./internal/captcha/... ./internal/shared/config/...
go build ./cmd/captcha-server
```

The application/domain boundary scan currently has no matches:

```bash
rg "internal/captcha/service|github.com/redis/go-redis|github.com/gin-gonic|google.golang.org/grpc|github.com/wenlng/go-captcha|internal/shared/config|net/http" internal/captcha/application internal/captcha/domain -n
```

## Remaining Work

Next low-risk tasks:

1. Refresh older runtime/image-source guide sections that still describe pre-refactor `service` ownership.
2. Refresh stats/proposal notes that still refer to `CaptchaService` or `TokenService` instrumentation points.
3. Run a final full validation pass after documentation cleanup.
4. Do not expand the test plan in this phase; keep using existing validation commands unless behavior changes require focused coverage.

## Fixed Decisions

These decisions remain aligned with the main plan:

1. `RuntimeImageSourceManager` belongs in `application`, not `domain`.
2. `ImageProviderFactory` is an application outbound port.
3. `ExternalImageProviderFactory` belongs in `adapter/outbound/image`.
4. `adapter/outbound/imagepool` remains the runtime adapter for image-pool behavior.
5. `internal/captcha/service` has been removed instead of kept as a compatibility facade.
6. HTTP package movement was completed as path cleanup after the core dependency direction stabilized.
