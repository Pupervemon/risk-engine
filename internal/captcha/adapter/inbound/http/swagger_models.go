package http

// ErrorResponseDoc documents the standard captcha error payload.
type ErrorResponseDoc struct {
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

// TrackPointDoc documents a single mouse-track point.
type TrackPointDoc struct {
	X int   `json:"x"`
	Y int   `json:"y"`
	T int64 `json:"t"`
}

// CaptchaChallengeResponseDoc documents the generated slider captcha payload.
type CaptchaChallengeResponseDoc struct {
	CaptchaID         string `json:"captchaId"`
	MasterImage       string `json:"masterImage"`
	TileImage         string `json:"tileImage"`
	TargetY           int    `json:"targetY"`
	ExpiresIn         int    `json:"expiresIn"`
	RequireMouseTrack bool   `json:"requireMouseTrack"`
}

// VerifyCaptchaRequestDoc documents the captcha verification request body.
type VerifyCaptchaRequestDoc struct {
	CaptchaID  string          `json:"captchaId"`
	PointX     int             `json:"pointX"`
	PointY     int             `json:"pointY"`
	MouseTrack []TrackPointDoc `json:"mouseTrack,omitempty"`
}

// VerifyCaptchaResponseDoc documents the token issued after captcha verification.
type VerifyCaptchaResponseDoc struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expiresIn"`
}

// HealthComponentDoc documents the health status of a single dependency.
type HealthComponentDoc struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthResponseDoc documents the service health response payload.
type HealthResponseDoc struct {
	Status     string                        `json:"status"`
	Components map[string]HealthComponentDoc `json:"components,omitempty"`
	Timestamp  string                        `json:"timestamp"`
}

// ProbeResponseDoc documents lightweight liveness-style probe responses.
type ProbeResponseDoc struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ReadinessResponseDoc documents readiness probe responses.
type ReadinessResponseDoc struct {
	Status     string                        `json:"status"`
	Timestamp  string                        `json:"timestamp"`
	Components map[string]HealthComponentDoc `json:"components,omitempty"`
}

// ImageSourceUpdateRequestDoc documents full update requests for the runtime image source.
type ImageSourceUpdateRequestDoc struct {
	URL                string `json:"url,omitempty"`
	APIKey             string `json:"apiKey,omitempty"`
	TimeoutSeconds     int    `json:"timeoutSeconds,omitempty"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute,omitempty"`
	RetryCount         int    `json:"retryCount,omitempty"`
	TriggerRefresh     bool   `json:"triggerRefresh,omitempty"`
}

// ImageSourceConfigDoc documents the sanitized runtime image source config.
type ImageSourceConfigDoc struct {
	Version            int64  `json:"version"`
	URL                string `json:"url"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

type ActiveImagePoolDoc struct {
	SourceConfigVersion int64  `json:"sourceConfigVersion"`
	SourceURL           string `json:"sourceURL,omitempty"`
	ImageCount          int64  `json:"imageCount"`
	RefreshedAt         string `json:"refreshedAt,omitempty"`
}

type ImageSourceSyncDoc struct {
	PoolSyncedWithConfig bool   `json:"poolSyncedWithConfig"`
	Message              string `json:"message"`
}

type ImageSourceOperationStatusDoc struct {
	LastValidatedAt     string `json:"lastValidatedAt,omitempty"`
	LastValidationError string `json:"lastValidationError,omitempty"`
	LastRefreshedAt     string `json:"lastRefreshedAt,omitempty"`
	LastRefreshError    string `json:"lastRefreshError,omitempty"`
}

type ImageSourceDebugDoc struct {
	ActiveGeneration string `json:"activeGeneration,omitempty"`
	GenerationCount  int64  `json:"generationCount"`
}

// ImageSourceStatusDoc documents the current runtime image source state.
type ImageSourceStatusDoc struct {
	Enabled    bool                          `json:"enabled"`
	Config     ImageSourceConfigDoc          `json:"config"`
	ActivePool ActiveImagePoolDoc            `json:"activePool"`
	Sync       ImageSourceSyncDoc            `json:"sync"`
	Status     ImageSourceOperationStatusDoc `json:"status"`
	PoolSize   int                           `json:"poolSize"`
	Debug      *ImageSourceDebugDoc          `json:"debug,omitempty"`
}

// ImageSourceValidationResultDoc documents the check endpoint response.
type ImageSourceValidationResultDoc struct {
	Config      ImageSourceConfigDoc `json:"config"`
	ValidatedAt string               `json:"validatedAt,omitempty"`
}

// ImageSourceOperationErrorResponseDoc documents error responses that also include a status snapshot.
type ImageSourceOperationErrorResponseDoc struct {
	Error  string               `json:"error"`
	Reason string               `json:"reason"`
	Status ImageSourceStatusDoc `json:"status"`
}
