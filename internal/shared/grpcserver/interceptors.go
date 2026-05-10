package grpcserver

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryTimeoutInterceptor 会给未显式设置截止时间的 unary 请求补一个默认超时。
func UnaryTimeoutInterceptor(defaultTimeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 先用带默认超时的上下文执行业务逻辑，避免请求长期悬挂。
		callCtx, cancel := withDefaultTimeout(ctx, defaultTimeout)
		defer cancel()

		resp, err := handler(callCtx, req)
		if err != nil {
			return resp, normalizeContextError(callCtx, err)
		}
		if callCtx.Err() != nil {
			return nil, normalizeContextError(callCtx, callCtx.Err())
		}
		return resp, nil
	}
}

// GracefulStop 先尝试优雅停止 gRPC 服务；如果外部上下文先结束，则强制停止。
func GracefulStop(ctx context.Context, server *grpc.Server) bool {
	done := make(chan struct{})
	go func() {
		// 优先等待正在处理的请求自然结束。
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		// 外部退出信号已到，直接强制停止，避免关停过程卡住。
		server.Stop()
		<-done
		return false
	}
}

// withDefaultTimeout 会在没有现成 deadline 时，为上下文补上默认超时。
func withDefaultTimeout(ctx context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	if defaultTimeout <= 0 {
		// 配置未启用超时，则直接复用原始上下文。
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		// 上游已经设置了 deadline，就不重复覆盖。
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// normalizeContextError 把 context 的取消/超时错误转换成更稳定的 gRPC 状态码。
func normalizeContextError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		// 统一返回 DEADLINE_EXCEEDED，方便调用方识别超时。
		return status.Error(codes.DeadlineExceeded, "DEADLINE_EXCEEDED")
	case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		// 统一返回 REQUEST_CANCELED，区分主动取消和其他错误。
		return status.Error(codes.Canceled, "REQUEST_CANCELED")
	default:
		return err
	}
}
