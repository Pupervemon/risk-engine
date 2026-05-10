package grpcserver

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type LogFieldExtractor func(req interface{}) []zap.Field

func UnaryLoggerInterceptor(logger *zap.Logger, extractors ...LogFieldExtractor) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = zap.NewNop()
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		if err != nil {
			logger.Error("rpc request failed", append(fields, zap.Error(err))...)
			return resp, err
		}

		for _, extract := range extractors {
			if extract == nil {
				continue
			}
			fields = append(fields, extract(req)...)
		}

		logger.Info("rpc request succeeded", fields...)
		return resp, nil
	}
}
