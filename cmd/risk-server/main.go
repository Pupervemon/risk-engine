package main

import (
	"context"
	"flag"
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
	configFile := flag.String("config", "", "path to config file")
	configEnv := flag.String("env", "", "app environment override")
	flag.Parse()

	cfg, err := config.LoadRiskConfigWithOptions(config.LoadOptions{
		ConfigPath: "configs",
		ConfigFile: *configFile,
		Env:        *configEnv,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("starting risk service",
		zap.Int("http_port", cfg.HTTP.Port),
		zap.Int("grpc_port", cfg.Grpc.Port),
		zap.Bool("nacos_enabled", cfg.Nacos.Enable))

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		logger.Fatal("failed to connect redis", zap.Error(err))
	}
	logger.Info("redis connected")

	riskService := riskservice.NewRiskService(rdb, &cfg.RiskRules, logger)

	httpRouter := risktransport.NewHealthRouter(rdb, logger, risktransport.ServiceInfo{
		Name:        cfg.Nacos.ServiceName,
		Version:     "1.0.0",
		Protocol:    "grpc",
		Description: "Risk Engine - risk control service",
		HTTPPort:    cfg.HTTP.Port,
		GRPCPort:    cfg.Grpc.Port,
	}, riskService)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      httpRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Grpc.Port))
	if err != nil {
		logger.Fatal("failed to listen grpc", zap.Error(err))
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryLoggerInterceptor(logger)),
	)

	pb.RegisterRiskControlServiceServer(grpcServer, riskService)
	reflection.Register(grpcServer)

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
		logger.Fatal("failed to initialize nacos registry", zap.Error(err))
	}

	go func() {
		logger.Info("risk http server listening", zap.Int("port", cfg.HTTP.Port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server exited unexpectedly", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("risk grpc server listening", zap.Int("port", cfg.Grpc.Port))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("grpc server exited unexpectedly", zap.Error(err))
		}
	}()

	time.Sleep(2 * time.Second)
	if err := nacosRegistry.Register(); err != nil {
		logger.Error("failed to register nacos service", zap.Error(err))
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	if err := nacosRegistry.Deregister(); err != nil {
		logger.Error("failed to deregister nacos service", zap.Error(err))
	}

	time.Sleep(1 * time.Second)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown http server", zap.Error(err))
	} else {
		logger.Info("http server closed")
	}

	grpcServer.GracefulStop()
	logger.Info("grpc server closed")
	logger.Info("risk service stopped")
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
			logger.Error("rpc request failed", append(fields, zap.Error(err))...)
			return resp, err
		}

		if r, ok := req.(*pb.CheckRequest); ok {
			fields = append(fields,
				zap.String("req_id", r.ReqId),
				zap.String("ip", r.Ip),
				zap.String("scene", r.Scene.String()),
			)
		}

		logger.Info("rpc request succeeded", fields...)
		return resp, nil
	}
}
