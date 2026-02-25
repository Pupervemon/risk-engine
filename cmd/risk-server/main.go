package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	riskservice "github.com/Pupervemon/risk-engine/internal/risk/service"
	risktransport "github.com/Pupervemon/risk-engine/internal/risk/transport"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/Pupervemon/risk-engine/internal/shared/registry"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {

	// 1. 加载配置
	cfg, err := config.LoadRiskConfig("configs")
	if err != nil {
		panic(fmt.Sprintf("无法加载配置: %v", err))
	}

	// 2. 初始化日志
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Risk服务启动中...",
		zap.Int("http_port", cfg.HTTP.Port),
		zap.Int("grpc_port", cfg.Grpc.Port),
		zap.Bool("nacos_enabled", cfg.Nacos.Enable))

	// 3. 初始化Redis连接
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second,
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second,
	})
	defer rdb.Close()

	// 测试Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		logger.Fatal("Redis 连接失败", zap.Error(err))
	}
	logger.Info("Redis 连接成功")

	// 4. 初始化HTTP健康检查服务（用于Nacos）
	httpRouter := risktransport.NewHealthRouter(rdb, logger)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 5. 初始化gRPC服务
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Grpc.Port))
	if err != nil {
		logger.Fatal("gRPC 监听失败", zap.Error(err))
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryLoggerInterceptor(logger)),
	)

	// 6. 注册风控服务
	riskService := riskservice.NewRiskService(rdb, &cfg.RiskRules, logger)
	pb.RegisterRiskControlServiceServer(grpcServer, riskService)
	reflection.Register(grpcServer)

	// 7. 初始化Nacos注册中心
	nacosRegistry, err := registry.NewNacosRegistry(&registry.NacosConfig{
		ServerAddr:  cfg.Nacos.ServerAddr,
		Namespace:   cfg.Nacos.Namespace,
		ServiceName: cfg.Nacos.ServiceName,
		GroupName:   cfg.Nacos.GroupName,
		ClusterName: cfg.Nacos.ClusterName,
		RegisterIP:  cfg.Nacos.RegisterIP,
		Weight:      cfg.Nacos.Weight,
		Enable:      cfg.Nacos.Enable,
		Metadata:    cfg.Nacos.Metadata,
		HttpPort:    cfg.HTTP.Port,
		GrpcPort:    cfg.Grpc.Port,
		HealthCheck: true,
	}, logger)
	if err != nil {
		logger.Fatal("初始化Nacos注册中心失败", zap.Error(err))
	}

	// 8. 启动HTTP服务（健康检查）
	go func() {
		logger.Info("Risk HTTP 健康检查服务启动", zap.Int("port", cfg.HTTP.Port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP 服务异常退出", zap.Error(err))
		}
	}()

	// 9. 启动gRPC服务
	go func() {
		logger.Info("Risk gRPC 服务启动", zap.Int("port", cfg.Grpc.Port))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("gRPC 服务异常退出", zap.Error(err))
		}
	}()

	// 10. 等待服务完全启动后注册到Nacos
	time.Sleep(2 * time.Second)
	if err := nacosRegistry.Register(); err != nil {
		logger.Error("注册到Nacos失败", zap.Error(err))
	}

	// 11. 优雅关闭处理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("收到退出信号，开始优雅关闭...", zap.String("signal", sig.String()))

	// 12. 从Nacos注销服务
	if err := nacosRegistry.Deregister(); err != nil {
		logger.Error("从Nacos注销服务失败", zap.Error(err))
	}

	// 13. 给一点时间让Nacos更新服务列表
	time.Sleep(1 * time.Second)

	// 14. 关闭HTTP服务器
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP服务器关闭失败", zap.Error(err))
	} else {
		logger.Info("HTTP服务器已关闭")
	}

	// 15. 关闭gRPC服务器
	grpcServer.GracefulStop()
	logger.Info("gRPC服务器已关闭")

	logger.Info("Risk服务已完全关闭")
}

// UnaryLoggerInterceptor - gRPC 日志拦截器
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

		// 对于 CheckRequest，记录关键信息
		if r, ok := req.(*pb.CheckRequest); ok {
			fields = append(fields,
				zap.String("req_id", r.ReqId),
				zap.String("ip", r.Ip),
				zap.String("scene", r.Scene.String()),
			)
		}

		logger.Info("RPC 请求成功", fields...)
		return resp, nil
	}
}
