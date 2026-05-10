package grpcserver

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
)

func TestUnaryLoggerInterceptorLogsSuccessWithExtractedFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	interceptor := UnaryLoggerInterceptor(logger, func(req interface{}) []zap.Field {
		return []zap.Field{zap.String("req_id", "req-1")}
	})

	resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/risk.v1.RiskControlService/Check"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}

	entries := logs.FilterMessage("rpc request succeeded").All()
	if len(entries) != 1 {
		t.Fatalf("expected one success log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["method"] != "/risk.v1.RiskControlService/Check" {
		t.Fatalf("unexpected method field: %v", fields["method"])
	}
	if fields["req_id"] != "req-1" {
		t.Fatalf("unexpected extracted req_id field: %v", fields["req_id"])
	}
}

func TestUnaryLoggerInterceptorLogsError(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	expectedErr := errors.New("handler failed")
	interceptor := UnaryLoggerInterceptor(logger, func(req interface{}) []zap.Field {
		t.Fatal("extractor should not run on failed requests")
		return nil
	})

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}

	entries := logs.FilterMessage("rpc request failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one error log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["method"] != "/svc/Method" {
		t.Fatalf("unexpected method field: %v", fields["method"])
	}
}
