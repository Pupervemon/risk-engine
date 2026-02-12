package http

import (
	"net/http"
	"time"

	"github.com/Pupervemon/risk-engine/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CaptchaHandler struct {
	CaptchaService *service.CaptchaService
	TokenService   *service.TokenService
	Logger         *zap.Logger
}

type captchaResponse struct {
	CaptchaID string `json:"captchaId"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expiresIn"`
}

type verifyRequest struct {
	CaptchaID   string `json:"captchaId"`
	CaptchaText string `json:"captchaText"`
}

type verifyResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expiresIn"`
}

type errorResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

func (h *CaptchaHandler) GetCaptcha(c *gin.Context) {
	captchaID, imageBase64, _, ttlSeconds, err := h.CaptchaService.Generate(c.Request.Context())
	if err != nil {
		h.Logger.Error("生成验证码失败", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "CAPTCHA_GENERATE_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	writeJSON(c, http.StatusOK, captchaResponse{
		CaptchaID: captchaID,
		Image:     imageBase64,
		ExpiresIn: ttlSeconds,
	})
}

func (h *CaptchaHandler) VerifyCaptcha(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	valid, reason, err := h.CaptchaService.Verify(c.Request.Context(), req.CaptchaID, req.CaptchaText)
	if err != nil {
		h.Logger.Error("验证码校验失败", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "CAPTCHA_VERIFY_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}
	if !valid {
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "CAPTCHA_INVALID", Reason: reason})
		return
	}

	token, exp, err := h.TokenService.IssueToken(req.CaptchaID)
	if err != nil {
		h.Logger.Error("签发 token 失败", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "TOKEN_ISSUE_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	expiresIn := exp - time.Now().Unix()
	if expiresIn < 0 {
		expiresIn = 0
	}

	writeJSON(c, http.StatusOK, verifyResponse{
		Token:     token,
		ExpiresIn: expiresIn,
	})
}

func writeJSON(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}
