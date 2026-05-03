package http

import (
	"net/http"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type CaptchaHandler struct {
	Captcha appports.CaptchaUseCase
	Token   appports.TokenUseCase
	Logger  *zap.Logger
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
	CaptchaID  string               `json:"captchaId"`
	PointX     int                  `json:"pointX"`
	PointY     int                  `json:"pointY"`
	MouseTrack *[]trackPointRequest `json:"mouseTrack,omitempty"` // Optional mouse-track data.
}

type trackPointRequest struct {
	X    int   `json:"x"`
	Y    int   `json:"y"`
	Time int64 `json:"t"`
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
	challenge, err := h.Captcha.Generate(c.Request.Context())
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

	result, err := h.Captcha.Verify(c.Request.Context(), appports.VerifyCaptchaCommand{
		CaptchaID:          req.CaptchaID,
		PointX:             req.PointX,
		PointY:             req.PointY,
		MouseTrack:         trackPointsToDomain(req.MouseTrack),
		MouseTrackProvided: req.MouseTrack != nil,
	})
	if err != nil {
		h.Logger.Error("captcha verification failed", zap.Error(err), zap.String("captchaId", req.CaptchaID))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "CAPTCHA_VERIFY_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}
	if !result.Valid {
		h.Logger.Warn("captcha rejected",
			zap.String("captchaId", req.CaptchaID),
			zap.Int("pointX", req.PointX),
			zap.String("reason", result.Reason))
		writeJSON(c, http.StatusBadRequest, errorResponse{Error: "CAPTCHA_INVALID", Reason: result.Reason})
		return
	}

	h.Logger.Info("captcha verified", zap.String("captchaId", req.CaptchaID))

	issuedToken, err := h.Token.Issue(c.Request.Context(), req.CaptchaID)
	if err != nil {
		h.Logger.Error("failed to issue token", zap.Error(err))
		writeJSON(c, http.StatusInternalServerError, errorResponse{Error: "TOKEN_ISSUE_FAILED", Reason: "INTERNAL_ERROR"})
		return
	}

	expiresIn := issuedToken.ExpiresAt - time.Now().Unix()
	if expiresIn < 0 {
		expiresIn = 0
	}

	writeJSON(c, http.StatusOK, verifyResponse{
		Token:     issuedToken.Token,
		ExpiresIn: expiresIn,
	})
}

func trackPointsToDomain(points *[]trackPointRequest) []domain.TrackPoint {
	if points == nil {
		return nil
	}

	converted := make([]domain.TrackPoint, 0, len(*points))
	for _, point := range *points {
		converted = append(converted, domain.TrackPoint{
			X:    point.X,
			Y:    point.Y,
			Time: point.Time,
		})
	}

	return converted
}

func writeJSON(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}
