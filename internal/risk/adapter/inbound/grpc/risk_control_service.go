package grpc

import (
	"context"
	"errors"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RiskControlService struct {
	pb.UnimplementedRiskControlServiceServer
	CheckUseCase     ports.RiskCheckUseCase
	EventUseCase     ports.RiskEventUseCase
	BlacklistUseCase ports.BlacklistUseCase
	ThrottleUseCase  ports.UserThrottleUseCase
}

func NewRiskControlService(
	check ports.RiskCheckUseCase,
	event ports.RiskEventUseCase,
	blacklist ports.BlacklistUseCase,
	throttle ports.UserThrottleUseCase,
) *RiskControlService {
	return &RiskControlService{
		CheckUseCase:     check,
		EventUseCase:     event,
		BlacklistUseCase: blacklist,
		ThrottleUseCase:  throttle,
	}
}

func (s *RiskControlService) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	decision, err := s.CheckUseCase.Check(ctx, checkCommandFromProto(req))
	if err != nil {
		return nil, err
	}

	return &pb.CheckResponse{Action: actionToProto(decision.Action), Reason: decision.Reason}, nil
}

func (s *RiskControlService) ReportEvent(ctx context.Context, req *pb.ReportEventRequest) (*pb.ReportEventResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	result, err := s.EventUseCase.ReportEvent(ctx, reportEventCommandFromProto(req))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUnsupportedScene):
			return nil, status.Error(codes.InvalidArgument, "UNSUPPORTED_SCENE")
		case errors.Is(err, domain.ErrLoginIdentityEmpty):
			return nil, status.Error(codes.InvalidArgument, "LOGIN_EVENT_IDENTITY_EMPTY")
		default:
			return nil, err
		}
	}

	return &pb.ReportEventResponse{Received: result.Received}, nil
}

func (s *RiskControlService) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) (*pb.AddBlacklistResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	result, err := s.BlacklistUseCase.AddBlacklist(ctx, addBlacklistCommandFromProto(req))
	if err != nil {
		return nil, err
	}

	return &pb.AddBlacklistResponse{Success: result.Success}, nil
}

func (s *RiskControlService) OnlineSelfTest(ctx context.Context, req *pb.OnlineSelfTestRequest) (*pb.OnlineSelfTestResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	result, err := s.ThrottleUseCase.OnlineSelfTest(ctx, onlineSelfTestCommandFromProto(req))
	if err != nil {
		return nil, userActionError(err)
	}

	return &pb.OnlineSelfTestResponse{Accepted: result.Accepted, Reason: result.Reason}, nil
}

func (s *RiskControlService) JudgeSubmission(ctx context.Context, req *pb.JudgeSubmissionRequest) (*pb.JudgeSubmissionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "REQUEST_EMPTY")
	}

	result, err := s.ThrottleUseCase.JudgeSubmission(ctx, judgeSubmissionCommandFromProto(req))
	if err != nil {
		return nil, userActionError(err)
	}

	return &pb.JudgeSubmissionResponse{Accepted: result.Accepted, Reason: result.Reason}, nil
}

func userActionError(err error) error {
	switch {
	case errors.Is(err, domain.ErrUserIDEmpty):
		return status.Error(codes.InvalidArgument, "USER_ID_EMPTY")
	case errors.Is(err, domain.ErrRateLimitExceeded):
		return status.Error(codes.ResourceExhausted, "USER_RATE_LIMIT_EXCEEDED")
	default:
		return err
	}
}
