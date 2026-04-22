package http

// swaggerGetCaptcha godoc
// @Summary Generate slider captcha
// @Description Returns a new slider captcha challenge, including master image, tile image, expiration, and whether mouse-track data is required.
// @Tags Captcha
// @Produce json
// @Success 200 {object} CaptchaChallengeResponseDoc
// @Failure 500 {object} ErrorResponseDoc
// @Router /api/v1/captcha [get]
func swaggerGetCaptcha() {}

// swaggerVerifyCaptcha godoc
// @Summary Verify slider captcha
// @Description Verifies the slider position and optional mouse-track payload. On success, returns a short-lived captcha token.
// @Tags Captcha
// @Accept json
// @Produce json
// @Param body body VerifyCaptchaRequestDoc true "Captcha verification payload. mouseTrack should be supplied when requireMouseTrack=true."
// @Success 200 {object} VerifyCaptchaResponseDoc
// @Failure 400 {object} ErrorResponseDoc
// @Failure 500 {object} ErrorResponseDoc
// @Router /api/v1/captcha/verify [post]
func swaggerVerifyCaptcha() {}

// swaggerGetImageSource godoc
// @Summary Get runtime image source status
// @Description Returns the current runtime image source configuration and latest validation and refresh state for the captcha image pool. `version` is the runtime config version, while `activeGeneration` is the currently exposed image-pool generation.
// @Tags Image Source Admin
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 3 (admin)."
// @Success 200 {object} ImageSourceStatusDoc
// @Failure 401 {object} ErrorResponseDoc
// @Failure 403 {object} ErrorResponseDoc
// @Failure 500 {object} ErrorResponseDoc
// @Router /api/v1/admin/image-source [get]
func swaggerGetImageSource() {}

// swaggerValidateImageSource godoc
// @Summary Validate runtime image source config
// @Description Validates a candidate upstream image source configuration without applying it.
// @Tags Image Source Admin
// @Accept json
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 3 (admin)."
// @Param body body ImageSourcePatchRequestDoc true "Candidate image source patch"
// @Success 200 {object} ImageSourceValidationResultDoc
// @Failure 400 {object} ErrorResponseDoc
// @Failure 401 {object} ErrorResponseDoc
// @Failure 403 {object} ErrorResponseDoc
// @Failure 409 {object} ErrorResponseDoc
// @Router /api/v1/admin/image-source/validate [post]
func swaggerValidateImageSource() {}

// swaggerUpdateImageSource godoc
// @Summary Update runtime image source config
// @Description Validates, persists, and applies a new runtime image source configuration, with optional immediate image-pool refresh.
// @Tags Image Source Admin
// @Accept json
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 3 (admin)."
// @Param body body ImageSourceUpdateRequestDoc true "Runtime image source update payload"
// @Success 200 {object} ImageSourceStatusDoc
// @Failure 400 {object} ImageSourceOperationErrorResponseDoc
// @Failure 401 {object} ErrorResponseDoc
// @Failure 403 {object} ErrorResponseDoc
// @Failure 409 {object} ErrorResponseDoc
// @Failure 500 {object} ImageSourceOperationErrorResponseDoc
// @Failure 502 {object} ImageSourceOperationErrorResponseDoc
// @Router /api/v1/admin/image-source [put]
func swaggerUpdateImageSource() {}

// swaggerRefreshImageSource godoc
// @Summary Refresh captcha image pool
// @Description Triggers an immediate refresh of the captcha image pool using the currently active runtime image source configuration.
// @Tags Image Source Admin
// @Produce json
// @Param X-User-Id header string true "Authenticated user ID injected by the gateway"
// @Param X-User-Roles header string true "Authenticated role list injected by the gateway. Supports comma-separated values or JSON array. Must include 3 (admin)."
// @Success 200 {object} ImageSourceStatusDoc
// @Failure 401 {object} ErrorResponseDoc
// @Failure 403 {object} ErrorResponseDoc
// @Failure 409 {object} ErrorResponseDoc
// @Failure 502 {object} ImageSourceOperationErrorResponseDoc
// @Router /api/v1/admin/image-source/refresh [post]
func swaggerRefreshImageSource() {}

// swaggerHealth godoc
// @Summary Service health check
// @Description Returns overall captcha service health and dependency status.
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponseDoc
// @Failure 503 {object} HealthResponseDoc
// @Router /health [get]
func swaggerHealth() {}

// swaggerDetailedHealth godoc
// @Summary Detailed actuator health check
// @Description Returns actuator-style health details, including dependency and disk probe status.
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponseDoc
// @Failure 503 {object} HealthResponseDoc
// @Router /actuator/health [get]
func swaggerDetailedHealth() {}

// swaggerLiveness godoc
// @Summary Liveness probe
// @Description Returns a lightweight liveness status for platform probes.
// @Tags Health
// @Produce json
// @Success 200 {object} ProbeResponseDoc
// @Router /actuator/health/liveness [get]
func swaggerLiveness() {}

// swaggerReadiness godoc
// @Summary Readiness probe
// @Description Returns readiness status and dependency details for platform probes.
// @Tags Health
// @Produce json
// @Success 200 {object} ReadinessResponseDoc
// @Failure 503 {object} ReadinessResponseDoc
// @Router /actuator/health/readiness [get]
func swaggerReadiness() {}
