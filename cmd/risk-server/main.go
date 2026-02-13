package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Pupervemon/risk-engine/internal/config"
	riskservice "github.com/Pupervemon/risk-engine/internal/risk/service"
	pb "github.com/Pupervemon/risk-proto/gen/go/risk/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // 引入 reflection
)

func main() {
	// 1. 初始化配置
	cfg, err := config.LoadConfig("configs") // 读取 config 目录下的配置文件
	if err != nil {
		panic(fmt.Sprintf("无法加载配置: %v", err))
	}

	// 2. 初始化日志 (Zap)
	logger, _ := zap.NewProduction()
	defer logger.Sync() // 确保日志在程序退出时被写入

	// 3. 初始化 Redis 连接 (使用配置文件)
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

	// 4. 监听 TCP 端口
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Grpc.Port))
	if err != nil {
		logger.Fatal("gRPC 监听失败", zap.Error(err))
	}

	// 5. 创建 gRPC 服务器
	s := grpc.NewServer(
		// 在这里可以添加 Interceptor (拦截器) 用于日志、监控等
		grpc.UnaryInterceptor(UnaryLoggerInterceptor(logger)),
	)

	// 6. 注册风控服务
	riskService := riskservice.NewRiskService(rdb, cfg.RiskRules, logger)
	pb.RegisterRiskControlServiceServer(s, riskService)

	// 7. 开启 gRPC 反射服务 (方便调试)
	reflection.Register(s)

	logger.Info("风控引擎启动成功", zap.Int("port", cfg.Grpc.Port))

	// 8. 启动服务
	if err := s.Serve(lis); err != nil {
		logger.Fatal("gRPC 服务启动失败", zap.Error(err))
	}
}

// UnaryLoggerInterceptor - gRPC 日志拦截器
func UnaryLoggerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		logFields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		if err != nil {
			logger.Error("RPC 请求失败", append(logFields, zap.Error(err))...)
		} else {
			// 对于 CheckRequest，可以记录关键信息
			if r, ok := req.(*pb.CheckRequest); ok {
				logFields = append(logFields, zap.String("req_id", r.ReqId), zap.String("ip", r.Ip))
			}
			logger.Info("RPC 请求成功", logFields...)
		}

		return resp, err
	}
}
