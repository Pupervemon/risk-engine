package http

import (
	"net/http"
	"time"

	captchaservice "github.com/Pupervemon/risk-engine/internal/captcha/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CaptchaHandler struct {
	CaptchaService *captchaservice.CaptchaService
	TokenService   *captchaservice.TokenService
	Logger         *zap.Logger
}

type captchaResponse struct {
	CaptchaID         string `json:"captchaId"`
	MasterImage       string `json:"masterImage"`
	TileImage         string `json:"tileImage"`
	TargetY           int    `json:"targetY"`
	ExpiresIn         int    `json:"expiresIn"`
	RequireMouseTrack bool   `json:"requireMouseTrack"` // Whether the client must submit mouse-track data.
}

type verifyRequest struct {
	CaptchaID  string                       `json:"captchaId"`
	PointX     int                          `json:"pointX"`
	PointY     int                          `json:"pointY"`
	MouseTrack *[]captchaservice.TrackPoint `json:"mouseTrack,omitempty"` // Optional mouse-track data.
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
	challenge, err := h.CaptchaService.Generate(c.Request.Context())
	if err != nil {
		h.Logger.Error("failed to generate captcha", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "CAPTCHA_GENERATE_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	writeJSON(c, http.StatusOK, captchaResponse{
		CaptchaID:         challenge.CaptchaID,
		MasterImage:       challenge.MasterImage,
		TileImage:         challenge.TileImage,
		TargetY:           challenge.TargetY,
		ExpiresIn:         challenge.ExpiresIn,
		RequireMouseTrack: challenge.RequireMouseTrack,
	})
}

func (h *CaptchaHandler) VerifyCaptcha(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.Logger.Warn("failed to parse captcha request body", zap.Error(err))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "INVALID_JSON", Reason: "BAD_REQUEST"})
		return
	}

	trackInfo := "absent"
	if req.MouseTrack != nil {
		trackInfo = "present"
	}

	h.Logger.Info("verifying captcha",
		zap.String("captchaId", req.CaptchaID),
		zap.Int("pointX", req.PointX),
		zap.Int("pointY", req.PointY),
		zap.String("track", trackInfo))

	valid, reason, err := h.CaptchaService.VerifyWithTrack(c.Request.Context(), req.CaptchaID, req.PointX, req.PointY, req.MouseTrack)
	if err != nil {
		h.Logger.Error("captcha verification failed", zap.Error(err), zap.String("captchaId", req.CaptchaID))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "CAPTCHA_VERIFY_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}
	if !valid {
		h.Logger.Warn("captcha rejected",
			zap.String("captchaId", req.CaptchaID),
			zap.Int("pointX", req.PointX),
			zap.String("reason", reason))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "CAPTCHA_INVALID", Reason: reason})
		return
	}

	h.Logger.Info("captcha verified", zap.String("captchaId", req.CaptchaID))

	token, exp, err := h.TokenService.IssueToken(c.Request.Context(), req.CaptchaID)
	if err != nil {
		h.Logger.Error("failed to issue token", zap.Error(err))
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
