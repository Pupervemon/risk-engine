package service

import (
	"context"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	captchapb "github.com/Pupervemon/risk-proto/gen/go/captcha/v1"
)

type CaptchaTokenService struct {
	captchapb.UnimplementedCaptchaTokenServiceServer
	Token appports.TokenUseCase
}

func NewCaptchaTokenService(token appports.TokenUseCase) *CaptchaTokenService {
	return &CaptchaTokenService{Token: token}
}

func (s *CaptchaTokenService) VerifyToken(ctx context.Context, req *captchapb.VerifyTokenRequest) (*captchapb.VerifyTokenResponse, error) {
	if req == nil || req.Token == "" {
		return &captchapb.VerifyTokenResponse{Valid: false, Reason: "TOKEN_EMPTY", ExpiresAt: 0}, nil
	}

	result, err := s.Token.Verify(ctx, req.Token)
	if err != nil {
		return &captchapb.VerifyTokenResponse{Valid: false, Reason: "TOKEN_VERIFY_FAILED", ExpiresAt: 0}, nil
	}
	return &captchapb.VerifyTokenResponse{Valid: result.Valid, Reason: result.Reason, ExpiresAt: result.ExpiresAt}, nil
}
