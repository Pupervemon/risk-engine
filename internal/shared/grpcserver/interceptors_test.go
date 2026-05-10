package grpcserver

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryTimeoutInterceptorAddsDeadlineWhenMissing(t *testing.T) {
	interceptor := UnaryTimeoutInterceptor(5 * time.Second)

	_, err := interceptor(context.Background(), nil, nil, func(ctx context.Context, req interface{}) (interface{}, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected server-side deadline")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnaryTimeoutInterceptorKeepsExistingDeadline(t *testing.T) {
	parentCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	interceptor := UnaryTimeoutInterceptor(time.Millisecond)

	_, err := interceptor(parentCtx, nil, nil, func(ctx context.Context, req interface{}) (interface{}, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected caller deadline")
		}
		if time.Until(deadline) < 30*time.Second {
			t.Fatalf("expected caller deadline to be preserved, got %s", time.Until(deadline))
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnaryTimeoutInterceptorCanBeDisabled(t *testing.T) {
	interceptor := UnaryTimeoutInterceptor(0)

	_, err := interceptor(context.Background(), nil, nil, func(ctx context.Context, req interface{}) (interface{}, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("expected no server-side deadline")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnaryTimeoutInterceptorMapsDeadlineExceeded(t *testing.T) {
	interceptor := UnaryTimeoutInterceptor(time.Millisecond)

	_, err := interceptor(context.Background(), nil, nil, func(ctx context.Context, req interface{}) (interface{}, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T %v", err, err)
	}
	if st.Code() != codes.DeadlineExceeded || st.Message() != "DEADLINE_EXCEEDED" {
		t.Fatalf("expected deadline exceeded status, got %s %q", st.Code(), st.Message())
	}
}
