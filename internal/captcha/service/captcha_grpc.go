package service

import (
	"context"

	captchapb "github.com/Pupervemon/risk-proto/gen/go/captcha/v1"
)

type CaptchaTokenService struct {
	captchapb.UnimplementedCaptchaTokenServiceServer
	TokenService *TokenService
}

func NewCaptchaTokenService(tokenService *TokenService) *CaptchaTokenService {
	return &CaptchaTokenService{TokenService: tokenService}
}

func (s *CaptchaTokenService) VerifyToken(ctx context.Context, req *captchapb.VerifyTokenRequest) (*captchapb.VerifyTokenResponse, error) {
	if req == nil || req.Token == "" {
		return &captchapb.VerifyTokenResponse{Valid: false, Reason: "TOKEN_EMPTY", ExpiresAt: 0}, nil
	}

	valid, reason, exp := s.TokenService.VerifyToken(ctx, req.Token)
	return &captchapb.VerifyTokenResponse{Valid: valid, Reason: reason, ExpiresAt: exp}, nil
}
