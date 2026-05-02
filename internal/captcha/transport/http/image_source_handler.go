package http

import (
	"errors"
	"net/http"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ImageSourceAdminHandler exposes runtime image-source administration APIs.
type ImageSourceAdminHandler struct {
	ImageSource appports.ImageSourceUseCase
	Logger      *zap.Logger
}

type imageSourcePatchPayload struct {
	URL                *string `json:"url"`
	APIKey             *string `json:"apiKey"`
	TimeoutSeconds     *int    `json:"timeoutSeconds"`
	RateLimitPerMinute *int    `json:"rateLimitPerMinute"`
	RetryCount         *int    `json:"retryCount"`
}

type imageSourceUpdatePayload struct {
	URL                *string `json:"url"`
	APIKey             *string `json:"apiKey"`
	TimeoutSeconds     *int    `json:"timeoutSeconds"`
	RateLimitPerMinute *int    `json:"rateLimitPerMinute"`
	RetryCount         *int    `json:"retryCount"`
	TriggerRefresh     *bool   `json:"triggerRefresh"`
}

type imageSourceConfigResponse struct {
	URL                string `json:"url"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
}

type imageSourceStatusResponse struct {
	Enabled             bool                      `json:"enabled"`
	Version             int64                     `json:"version"`
	Config              imageSourceConfigResponse `json:"config"`
	UpdatedAt           string                    `json:"updatedAt,omitempty"`
	LastValidatedAt     string                    `json:"lastValidatedAt,omitempty"`
	LastValidationError string                    `json:"lastValidationError,omitempty"`
	LastRefreshedAt     string                    `json:"lastRefreshedAt,omitempty"`
	LastRefreshError    string                    `json:"lastRefreshError,omitempty"`
	PoolSize            int                       `json:"poolSize"`
	PoolImageCount      int64                     `json:"poolImageCount"`
	ActiveGeneration    string                    `json:"activeGeneration,omitempty"`
	GenerationCount     int64                     `json:"generationCount"`
}

type imageSourceValidationResponse struct {
	Config      imageSourceConfigResponse `json:"config"`
	ValidatedAt string                    `json:"validatedAt,omitempty"`
}

func (h *ImageSourceAdminHandler) GetImageSource(c *gin.Context) {
	status, err := h.ImageSource.Status(c.Request.Context())
	if err != nil {
		h.Logger.Error("failed to get image source status", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "IMAGE_SOURCE_STATUS_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	writeJSON(c, http.StatusOK, imageSourceStatusResponseFromDomain(status))
}

func (h *ImageSourceAdminHandler) ValidateImageSource(c *gin.Context) {
	var req imageSourcePatchPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("failed to parse image source validate request", zap.Error(err))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	result, err := h.ImageSource.Validate(c.Request.Context(), buildImageSourcePatch(req))
	if err != nil {
		h.writeImageSourceError(c, err, "IMAGE_SOURCE_VALIDATE_FAILED")
		return
	}

	writeJSON(c, http.StatusOK, imageSourceValidationResponseFromDomain(result))
}

func (h *ImageSourceAdminHandler) UpdateImageSource(c *gin.Context) {
	var req imageSourceUpdatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("failed to parse image source update request", zap.Error(err))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	triggerRefresh := true
	if req.TriggerRefresh != nil {
		triggerRefresh = *req.TriggerRefresh
	}

	status, err := h.ImageSource.Update(c.Request.Context(), buildImageSourcePatch(imageSourcePatchPayload{
		URL:                req.URL,
		APIKey:             req.APIKey,
		TimeoutSeconds:     req.TimeoutSeconds,
		RateLimitPerMinute: req.RateLimitPerMinute,
		RetryCount:         req.RetryCount,
	}), triggerRefresh)
	if err != nil {
		h.writeImageSourceUpdateError(c, err, status)
		return
	}

	writeJSON(c, http.StatusOK, imageSourceStatusResponseFromDomain(status))
}

func (h *ImageSourceAdminHandler) RefreshImageSource(c *gin.Context) {
	status, err := h.ImageSource.Refresh(c.Request.Context())
	if err != nil {
		h.Logger.Warn("failed to refresh image pool", zap.Error(err))
		if errors.Is(err, domain.ErrImagePoolDisabled) {
			writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
			return
		}
		writeJSON(c, http.StatusBadGateway, gin.H{
			"error":  "IMAGE_POOL_REFRESH_FAILED",
			"reason": err.Error(),
			"status": imageSourceStatusResponseFromDomain(status),
		})
		return
	}

	writeJSON(c, http.StatusOK, imageSourceStatusResponseFromDomain(status))
}

func (h *ImageSourceAdminHandler) writeImageSourceError(c *gin.Context, err error, code string) {
	h.Logger.Warn("image source operation failed", zap.Error(err))
	if errors.Is(err, domain.ErrImagePoolDisabled) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
		return
	}
	if errors.Is(err, domain.ErrImagePoolRefreshInProgress) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_REFRESH_IN_PROGRESS", Reason: err.Error()})
		return
	}

	writeJSON(c, http.StatusBadRequest, errorResponse{Error: code, Reason: err.Error()})
}

func (h *ImageSourceAdminHandler) writeImageSourceUpdateError(c *gin.Context, err error, status domain.ImageSourceStatus) {
	h.Logger.Warn("image source update failed", zap.Error(err))
	if errors.Is(err, domain.ErrImagePoolDisabled) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
		return
	}
	if errors.Is(err, domain.ErrImagePoolRefreshInProgress) {
		writeJSON(c, http.StatusConflict, gin.H{
			"error":  "IMAGE_POOL_REFRESH_IN_PROGRESS",
			"reason": err.Error(),
			"status": imageSourceStatusResponseFromDomain(status),
		})
		return
	}

	var refreshErr *domain.ImageSourceRefreshError
	if errors.As(err, &refreshErr) {
		writeJSON(c, http.StatusBadGateway, gin.H{
			"error":  "IMAGE_SOURCE_REFRESH_FAILED",
			"reason": err.Error(),
			"status": imageSourceStatusResponseFromDomain(status),
		})
		return
	}

	var persistenceErr *domain.ImageSourcePersistenceError
	if errors.As(err, &persistenceErr) {
		writeJSON(c, http.StatusInternalServerError, gin.H{
			"error":  "IMAGE_SOURCE_PERSIST_FAILED",
			"reason": err.Error(),
			"status": imageSourceStatusResponseFromDomain(status),
		})
		return
	}

	writeJSON(c, http.StatusBadRequest, gin.H{
		"error":  "IMAGE_SOURCE_UPDATE_REJECTED",
		"reason": err.Error(),
		"status": imageSourceStatusResponseFromDomain(status),
	})
}

func buildImageSourcePatch(req imageSourcePatchPayload) domain.ImageSourcePatch {
	return domain.ImageSourcePatch{
		URL:                req.URL,
		APIKey:             req.APIKey,
		TimeoutSeconds:     req.TimeoutSeconds,
		RateLimitPerMinute: req.RateLimitPerMinute,
		RetryCount:         req.RetryCount,
	}
}

func imageSourceStatusResponseFromDomain(status domain.ImageSourceStatus) imageSourceStatusResponse {
	return imageSourceStatusResponse{
		Enabled:             status.Enabled,
		Version:             status.Version,
		Config:              imageSourceConfigResponseFromDomain(status.Config),
		UpdatedAt:           status.UpdatedAt,
		LastValidatedAt:     status.LastValidatedAt,
		LastValidationError: status.LastValidationError,
		LastRefreshedAt:     status.LastRefreshedAt,
		LastRefreshError:    status.LastRefreshError,
		PoolSize:            status.PoolSize,
		PoolImageCount:      status.PoolImageCount,
		ActiveGeneration:    status.ActiveGeneration,
		GenerationCount:     status.GenerationCount,
	}
}

func imageSourceValidationResponseFromDomain(result domain.ImageSourceValidationResult) imageSourceValidationResponse {
	return imageSourceValidationResponse{
		Config:      imageSourceConfigResponseFromDomain(result.Config),
		ValidatedAt: result.ValidatedAt,
	}
}

func imageSourceConfigResponseFromDomain(config domain.ImageSourceConfigView) imageSourceConfigResponse {
	return imageSourceConfigResponse{
		URL:                config.URL,
		APIKeyConfigured:   config.APIKeyConfigured,
		TimeoutSeconds:     config.TimeoutSeconds,
		RateLimitPerMinute: config.RateLimitPerMinute,
		RetryCount:         config.RetryCount,
	}
}
