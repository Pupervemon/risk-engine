package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeRiskUseCases struct {
	checkCommand        ports.CheckCommand
	checkDecision       ports.CheckDecision
	checkErr            error
	reportCommand       ports.ReportEventCommand
	reportResult        ports.ReportEventResult
	reportErr           error
	addBlacklistCommand ports.AddBlacklistCommand
	addBlacklistResult  ports.AddBlacklistResult
	addBlacklistErr     error
	onlineCommand       ports.UserActionCommand
	onlineResult        ports.UserActionResult
	onlineErr           error
	judgeCommand        ports.UserActionCommand
	judgeResult         ports.UserActionResult
	judgeErr            error
}

func (f *fakeRiskUseCases) Check(_ context.Context, cmd ports.CheckCommand) (ports.CheckDecision, error) {
	f.checkCommand = cmd
	return f.checkDecision, f.checkErr
}

func (f *fakeRiskUseCases) ReportEvent(_ context.Context, cmd ports.ReportEventCommand) (ports.ReportEventResult, error) {
	f.reportCommand = cmd
	return f.reportResult, f.reportErr
}

func (f *fakeRiskUseCases) AddBlacklist(_ context.Context, cmd ports.AddBlacklistCommand) (ports.AddBlacklistResult, error) {
	f.addBlacklistCommand = cmd
	return f.addBlacklistResult, f.addBlacklistErr
}

func (f *fakeRiskUseCases) OnlineSelfTest(_ context.Context, cmd ports.UserActionCommand) (ports.UserActionResult, error) {
	f.onlineCommand = cmd
	return f.onlineResult, f.onlineErr
}

func (f *fakeRiskUseCases) JudgeSubmission(_ context.Context, cmd ports.UserActionCommand) (ports.UserActionResult, error) {
	f.judgeCommand = cmd
	return f.judgeResult, f.judgeErr
}

func TestRiskControlServiceCheckMapsRequestAndResponse(t *testing.T) {
	fake := &fakeRiskUseCases{
		checkDecision: ports.CheckDecision{
			Action: domain.ActionVerify,
			Reason: "TOO_MANY_FAILED_ATTEMPTS",
		},
	}
	service := NewRiskControlService(fake, fake, fake, fake)

	resp, err := service.Check(context.Background(), &pb.CheckRequest{
		ReqId:       "req-1",
		Scene:       pb.Scene_SCENE_LOGIN,
		Ip:          "127.0.0.1",
		UserId:      "user-1",
		PhoneNumber: "13800000000",
		DeviceId:    "device-1",
		Timestamp:   123,
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if resp.Action != pb.Action_ACTION_VERIFY || resp.Reason != "TOO_MANY_FAILED_ATTEMPTS" {
		t.Fatalf("unexpected response: action=%v reason=%q", resp.Action, resp.Reason)
	}
	if fake.checkCommand.ReqID != "req-1" ||
		fake.checkCommand.Scene != domain.SceneLogin ||
		fake.checkCommand.IP != "127.0.0.1" ||
		fake.checkCommand.UserID != "user-1" ||
		fake.checkCommand.PhoneNumber != "13800000000" ||
		fake.checkCommand.DeviceID != "device-1" ||
		fake.checkCommand.Timestamp != 123 {
		t.Fatalf("unexpected mapped check command: %+v", fake.checkCommand)
	}
}

func TestRiskControlServiceCheckRejectsEmptyRequest(t *testing.T) {
	service := NewRiskControlService(&fakeRiskUseCases{}, &fakeRiskUseCases{}, &fakeRiskUseCases{}, &fakeRiskUseCases{})

	_, err := service.Check(context.Background(), nil)
	assertStatus(t, err, codes.InvalidArgument, "REQUEST_EMPTY")
}

func TestRiskControlServiceReportEventMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{name: "unsupported scene", err: domain.ErrUnsupportedScene, message: "UNSUPPORTED_SCENE"},
		{name: "empty identity", err: domain.ErrLoginIdentityEmpty, message: "LOGIN_EVENT_IDENTITY_EMPTY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRiskUseCases{reportErr: tt.err}
			service := NewRiskControlService(fake, fake, fake, fake)

			_, err := service.ReportEvent(context.Background(), &pb.ReportEventRequest{Scene: pb.Scene_SCENE_REGISTER})
			assertStatus(t, err, codes.InvalidArgument, tt.message)
		})
	}
}

func TestRiskControlServiceReportEventSuccess(t *testing.T) {
	fake := &fakeRiskUseCases{reportResult: ports.ReportEventResult{Received: true}}
	service := NewRiskControlService(fake, fake, fake, fake)

	resp, err := service.ReportEvent(context.Background(), &pb.ReportEventRequest{
		ReqId:     "req-2",
		Scene:     pb.Scene_SCENE_LOGIN,
		Ip:        "127.0.0.1",
		UserId:    "user-2",
		IsSuccess: false,
		ExtraInfo: "password failed",
	})
	if err != nil {
		t.Fatalf("ReportEvent returned error: %v", err)
	}
	if !resp.Received {
		t.Fatal("expected report event response to be received")
	}
	if fake.reportCommand.Scene != domain.SceneLogin ||
		fake.reportCommand.IP != "127.0.0.1" ||
		fake.reportCommand.UserID != "user-2" ||
		fake.reportCommand.ExtraInfo != "password failed" {
		t.Fatalf("unexpected mapped report command: %+v", fake.reportCommand)
	}
}

func TestRiskControlServiceAddBlacklistMapsRequest(t *testing.T) {
	fake := &fakeRiskUseCases{addBlacklistResult: ports.AddBlacklistResult{Success: true}}
	service := NewRiskControlService(fake, fake, fake, fake)

	resp, err := service.AddBlacklist(context.Background(), &pb.AddBlacklistRequest{
		Type:     pb.AddBlacklistRequest_TYPE_USER_ID,
		Value:    "user-3",
		Reason:   "manual",
		ExpireAt: 456,
	})
	if err != nil {
		t.Fatalf("AddBlacklist returned error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected add blacklist response success")
	}
	if fake.addBlacklistCommand.Type != domain.BlacklistTypeUserID ||
		fake.addBlacklistCommand.Value != "user-3" ||
		fake.addBlacklistCommand.Reason != "manual" ||
		fake.addBlacklistCommand.ExpireAt != 456 {
		t.Fatalf("unexpected mapped blacklist command: %+v", fake.addBlacklistCommand)
	}
}

func TestRiskControlServiceUserActionErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{name: "user id empty", err: domain.ErrUserIDEmpty, code: codes.InvalidArgument, message: "USER_ID_EMPTY"},
		{name: "rate limited", err: domain.ErrRateLimitExceeded, code: codes.ResourceExhausted, message: "USER_RATE_LIMIT_EXCEEDED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRiskUseCases{onlineErr: tt.err}
			service := NewRiskControlService(fake, fake, fake, fake)

			_, err := service.OnlineSelfTest(context.Background(), &pb.OnlineSelfTestRequest{UserId: "user-4"})
			assertStatus(t, err, tt.code, tt.message)
		})
	}
}

func TestRiskControlServiceJudgeSubmissionSuccess(t *testing.T) {
	fake := &fakeRiskUseCases{judgeResult: ports.UserActionResult{Accepted: true, Reason: "PASS"}}
	service := NewRiskControlService(fake, fake, fake, fake)

	resp, err := service.JudgeSubmission(context.Background(), &pb.JudgeSubmissionRequest{
		ReqId:     "req-5",
		UserId:    "user-5",
		Answer:    "42",
		Timestamp: 789,
	})
	if err != nil {
		t.Fatalf("JudgeSubmission returned error: %v", err)
	}
	if !resp.Accepted || resp.Reason != "PASS" {
		t.Fatalf("unexpected judge response: %+v", resp)
	}
	if fake.judgeCommand.UserID != "user-5" || fake.judgeCommand.Answer != "42" || fake.judgeCommand.Timestamp != 789 {
		t.Fatalf("unexpected mapped judge command: %+v", fake.judgeCommand)
	}
}

func TestRiskControlServicePreservesUnknownErrors(t *testing.T) {
	expectedErr := errors.New("storage failed")
	fake := &fakeRiskUseCases{checkErr: expectedErr}
	service := NewRiskControlService(fake, fake, fake, fake)

	_, err := service.Check(context.Background(), &pb.CheckRequest{Ip: "127.0.0.1"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
}

func assertStatus(t *testing.T, err error, expectedCode codes.Code, expectedMessage string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected status error %s %q, got nil", expectedCode, expectedMessage)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T %v", err, err)
	}
	if st.Code() != expectedCode || st.Message() != expectedMessage {
		t.Fatalf("expected status %s %q, got %s %q", expectedCode, expectedMessage, st.Code(), st.Message())
	}
}
