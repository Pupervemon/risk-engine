package grpc

import (
	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
)

func sceneFromProto(scene pb.Scene) domain.Scene {
	switch scene {
	case pb.Scene_SCENE_LOGIN:
		return domain.SceneLogin
	case pb.Scene_SCENE_REGISTER:
		return domain.SceneRegister
	case pb.Scene_SCENE_PAYMENT:
		return domain.ScenePayment
	default:
		return domain.SceneUnknown
	}
}

func actionToProto(action domain.Action) pb.Action {
	switch action {
	case domain.ActionReject:
		return pb.Action_ACTION_REJECT
	case domain.ActionVerify:
		return pb.Action_ACTION_VERIFY
	default:
		return pb.Action_ACTION_PASS
	}
}

func blacklistTypeFromProto(entryType pb.AddBlacklistRequest_BlacklistType) domain.BlacklistType {
	switch entryType {
	case pb.AddBlacklistRequest_TYPE_IP:
		return domain.BlacklistTypeIP
	case pb.AddBlacklistRequest_TYPE_USER_ID:
		return domain.BlacklistTypeUserID
	default:
		return domain.BlacklistTypeUnknown
	}
}

func checkCommandFromProto(req *pb.CheckRequest) ports.CheckCommand {
	return ports.CheckCommand{
		ReqID:       req.ReqId,
		Scene:       sceneFromProto(req.Scene),
		IP:          req.Ip,
		UserID:      req.UserId,
		PhoneNumber: req.PhoneNumber,
		DeviceID:    req.DeviceId,
		Timestamp:   req.Timestamp,
	}
}

func reportEventCommandFromProto(req *pb.ReportEventRequest) ports.ReportEventCommand {
	return ports.ReportEventCommand{
		ReqID:     req.ReqId,
		Scene:     sceneFromProto(req.Scene),
		IP:        req.Ip,
		UserID:    req.UserId,
		IsSuccess: req.IsSuccess,
		ExtraInfo: req.ExtraInfo,
	}
}

func addBlacklistCommandFromProto(req *pb.AddBlacklistRequest) ports.AddBlacklistCommand {
	return ports.AddBlacklistCommand{
		Type:     blacklistTypeFromProto(req.Type),
		Value:    req.Value,
		Reason:   req.Reason,
		ExpireAt: req.ExpireAt,
	}
}

func onlineSelfTestCommandFromProto(req *pb.OnlineSelfTestRequest) ports.UserActionCommand {
	return ports.UserActionCommand{
		ReqID:     req.ReqId,
		UserID:    req.UserId,
		Payload:   req.Payload,
		Timestamp: req.Timestamp,
	}
}

func judgeSubmissionCommandFromProto(req *pb.JudgeSubmissionRequest) ports.UserActionCommand {
	return ports.UserActionCommand{
		ReqID:     req.ReqId,
		UserID:    req.UserId,
		Answer:    req.Answer,
		Timestamp: req.Timestamp,
	}
}
