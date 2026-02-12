package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Pupervemon/risk-engine/internal/config"
	"github.com/Pupervemon/risk-engine/internal/service"
	httptransport "github.com/Pupervemon/risk-engine/internal/transport/http"
	captchapb "github.com/Pupervemon/risk-proto/gen/go/captcha/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg, err := config.LoadCaptchaConfig("configs")
	if err != nil {
		panic(fmt.Sprintf("无法加载配置: %v", err))
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second,
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		logger.Fatal("Redis 连接失败", zap.Error(err))
	}
	logger.Info("Redis 连接成功")

	captchaService := service.NewCaptchaService(rdb, &cfg.Captcha, logger)
	tokenService := service.NewTokenService(&cfg.Token)
	grpcService := service.NewCaptchaTokenService(tokenService)

	httpHandler := &httptransport.CaptchaHandler{
		CaptchaService: captchaService,
		TokenService:   tokenService,
		Logger:         logger,
	}
	router := httptransport.NewCaptchaRouter(httpHandler)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler: router,
	}

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Grpc.Port))
	if err != nil {
		logger.Fatal("gRPC 监听失败", zap.Error(err))
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(UnaryLoggerInterceptor(logger)))
	captchapb.RegisterCaptchaTokenServiceServer(grpcServer, grpcService)
	reflection.Register(grpcServer)

	errCh := make(chan error, 2)

	go func() {
		logger.Info("Captcha HTTP 服务启动成功", zap.Int("port", cfg.HTTP.Port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	go func() {
		logger.Info("Captcha gRPC 服务启动成功", zap.Int("port", cfg.Grpc.Port))
		if err := grpcServer.Serve(grpcListener); err != nil {
			errCh <- err
		}
	}()

	err = <-errCh
	logger.Fatal("服务异常退出", zap.Error(err))
}

func UnaryLoggerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		if err != nil {
			logger.Error("RPC 请求失败", append(fields, zap.Error(err))...)
			return resp, err
		}

		logger.Info("RPC 请求成功", fields...)
		return resp, nil
	}
}
