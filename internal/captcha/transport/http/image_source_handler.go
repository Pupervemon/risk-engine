package http

import (
	"errors"
	"net/http"

	captchaservice "github.com/Pupervemon/risk-engine/internal/captcha/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ImageSourceAdminHandler 负责暴露图片源的管理接口。
//
// 这一层只做 HTTP 请求解析、错误映射和响应输出，真正的业务处理
// 都委托给 captcha service 完成。
type ImageSourceAdminHandler struct {
	CaptchaService *captchaservice.CaptchaService
	Logger         *zap.Logger
}

// imageSourcePatchPayload 用于局部更新和校验图片源配置。
//
// 所有字段都使用指针，是为了区分“请求里没有传这个字段”和“显式传了零值”。
// 这样服务层就可以根据是否为 nil 来决定是否覆盖对应配置项。
type imageSourcePatchPayload struct {
	URL                *string `json:"url"`
	APIKey             *string `json:"apiKey"`
	TimeoutSeconds     *int    `json:"timeoutSeconds"`
	RateLimitPerMinute *int    `json:"rateLimitPerMinute"`
	RetryCount         *int    `json:"retryCount"`
}

// imageSourceUpdatePayload 是更新接口的完整请求体。
//
// 它在 patch payload 的基础上增加了 TriggerRefresh，允许调用方决定：
// 更新配置后是否立即刷新图片池。
type imageSourceUpdatePayload struct {
	URL                *string `json:"url"`
	APIKey             *string `json:"apiKey"`
	TimeoutSeconds     *int    `json:"timeoutSeconds"`
	RateLimitPerMinute *int    `json:"rateLimitPerMinute"`
	RetryCount         *int    `json:"retryCount"`
	TriggerRefresh     *bool   `json:"triggerRefresh"`
}

// GetImageSource 返回当前图片源的运行时状态。
//
// 典型用途是让管理端查看当前是否启用了图片池、最近一次刷新结果等信息。
func (h *ImageSourceAdminHandler) GetImageSource(c *gin.Context) {
	status, err := h.CaptchaService.GetImageSourceStatus(c.Request.Context())
	if err != nil {
		// 状态读取失败通常意味着底层服务或运行时状态不可用，直接返回 500。
		h.Logger.Error("failed to get image source status", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "IMAGE_SOURCE_STATUS_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	writeJSON(c, http.StatusOK, status)
}

// ValidateImageSource 校验传入的图片源参数，但不真正落库或刷新运行时配置。
//
// 这个接口适合在修改配置前做预检，避免用户提交后才发现地址、超时、
// 限流等参数不合法。
func (h *ImageSourceAdminHandler) ValidateImageSource(c *gin.Context) {
	var req imageSourcePatchPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		// JSON 解析失败属于客户端请求格式错误，返回 400。
		h.Logger.Warn("failed to parse image source validate request", zap.Error(err))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	// 将请求体转换为服务层使用的 patch 结构，保持 transport 层只负责协议适配。
	result, err := h.CaptchaService.ValidateImageSource(c.Request.Context(), buildImageSourcePatch(req))
	if err != nil {
		h.writeImageSourceError(c, err, "IMAGE_SOURCE_VALIDATE_FAILED")
		return
	}

	writeJSON(c, http.StatusOK, result)
}

// UpdateImageSource 更新图片源配置，并根据请求决定是否立即刷新图片池。
//
// 默认行为是更新后刷新，这样管理端修改完配置后可以立刻生效；
// 如果 TriggerRefresh 显式传 false，则只更新配置，不触发刷新。
func (h *ImageSourceAdminHandler) UpdateImageSource(c *gin.Context) {
	var req imageSourceUpdatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		// 更新接口同样把 JSON 格式错误当作客户端问题处理。
		h.Logger.Warn("failed to parse image source update request", zap.Error(err))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	// 默认更新后自动刷新，除非调用方明确关闭。
	triggerRefresh := true
	if req.TriggerRefresh != nil {
		triggerRefresh = *req.TriggerRefresh
	}

	// 这里复用 patch 结构，避免为 update 和 validate 维护两套重复字段。
	status, err := h.CaptchaService.UpdateImageSource(c.Request.Context(), buildImageSourcePatch(imageSourcePatchPayload{
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

	writeJSON(c, http.StatusOK, status)
}

// RefreshImageSource 强制刷新当前图片池。
//
// 这个接口主要给运维或管理端使用，用于在外部资源变更后手动触发重载。
func (h *ImageSourceAdminHandler) RefreshImageSource(c *gin.Context) {
	status, err := h.CaptchaService.RefreshImagePool(c.Request.Context())
	if err != nil {
		// 刷新失败时区分两类场景：
		// 1. 图片池未启用，属于业务状态冲突，返回 409。
		// 2. 其他刷新失败，说明上游资源或运行时异常，返回 502。
		h.Logger.Warn("failed to refresh image pool", zap.Error(err))
		if errors.Is(err, captchaservice.ErrImagePoolDisabled) {
			writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
			return
		}
		writeJSON(c, http.StatusBadGateway, gin.H{
			"error":  "IMAGE_POOL_REFRESH_FAILED",
			"reason": err.Error(),
			"status": status,
		})
		return
	}

	writeJSON(c, http.StatusOK, status)
}

// writeImageSourceError 统一处理图片源校验/读取类错误。
//
// 这里把“图片池未启用”映射为 409，其余错误映射为 400，方便前端
// 根据状态码判断是配置冲突还是参数本身不合法。
func (h *ImageSourceAdminHandler) writeImageSourceError(c *gin.Context, err error, code string) {
	h.Logger.Warn("image source operation failed", zap.Error(err))
	if errors.Is(err, captchaservice.ErrImagePoolDisabled) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
		return
	}
	if errors.Is(err, captchaservice.ErrImagePoolRefreshInProgress) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_REFRESH_IN_PROGRESS", Reason: err.Error()})
		return
	}

	writeJSON(c, http.StatusBadRequest, errorResponse{Error: code, Reason: err.Error()})
}

// writeImageSourceUpdateError 统一处理更新类错误。
//
// 更新和刷新属于更强的副作用操作，所以除“图片池未启用”外，
// 其余失败更适合返回 502，表示上游依赖或运行时刷新过程失败。
func (h *ImageSourceAdminHandler) writeImageSourceUpdateError(c *gin.Context, err error, status captchaservice.ImageSourceStatus) {
	h.Logger.Warn("image source update failed", zap.Error(err))
	if errors.Is(err, captchaservice.ErrImagePoolDisabled) {
		writeJSON(c, http.StatusConflict, errorResponse{Error: "IMAGE_POOL_DISABLED", Reason: err.Error()})
		return
	}
	if errors.Is(err, captchaservice.ErrImagePoolRefreshInProgress) {
		writeJSON(c, http.StatusConflict, gin.H{
			"error":  "IMAGE_POOL_REFRESH_IN_PROGRESS",
			"reason": err.Error(),
			"status": status,
		})
		return
	}

	var refreshErr *captchaservice.ImageSourceRefreshError
	if errors.As(err, &refreshErr) {
		writeJSON(c, http.StatusBadGateway, gin.H{
			"error":  "IMAGE_SOURCE_REFRESH_FAILED",
			"reason": err.Error(),
			"status": status,
		})
		return
	}

	var persistenceErr *captchaservice.ImageSourcePersistenceError
	if errors.As(err, &persistenceErr) {
		writeJSON(c, http.StatusInternalServerError, gin.H{
			"error":  "IMAGE_SOURCE_PERSIST_FAILED",
			"reason": err.Error(),
			"status": status,
		})
		return
	}

	writeJSON(c, http.StatusBadRequest, gin.H{
		"error":  "IMAGE_SOURCE_UPDATE_REJECTED",
		"reason": err.Error(),
		"status": status,
	})
}

// buildImageSourcePatch 将 transport 层的 JSON 请求体转换为 service 层 patch。
//
// 这个转换函数看起来简单，但它的作用是把协议层字段和业务层字段解耦，
// 以后如果 HTTP 结构需要调整，服务层通常不需要跟着改。
func buildImageSourcePatch(req imageSourcePatchPayload) captchaservice.ImageSourcePatch {
	return captchaservice.ImageSourcePatch{
		URL:                req.URL,
		APIKey:             req.APIKey,
		TimeoutSeconds:     req.TimeoutSeconds,
		RateLimitPerMinute: req.RateLimitPerMinute,
		RetryCount:         req.RetryCount,
	}
}
