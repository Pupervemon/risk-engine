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

	grpcadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/inbound/grpc"
	httptransport "github.com/Pupervemon/risk-engine/internal/captcha/adapter/inbound/http"
	captchabootstrap "github.com/Pupervemon/risk-engine/internal/captcha/bootstrap"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/Pupervemon/risk-engine/internal/shared/logging"
	"github.com/Pupervemon/risk-engine/internal/shared/registry"
	captchapb "github.com/Pupervemon/risk-proto/gen/go/captcha/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// To export Swagger docs later, run from the repo root:
// swag init --parseInternal --outputTypes json,yaml --dir cmd/captcha-server,internal/captcha/adapter/inbound/http -g main.go -o docs/swagger/captcha
//
// @title Captcha Service HTTP API
// @version 1.0.0
// @description HTTP endpoints for captcha generation, verification, health checks, and runtime image-source administration.
// @BasePath /
// @schemes http
// @tag.name Captcha
// @tag.description Endpoints for generating and verifying slider captchas.
// @tag.name Image Source Admin
// @tag.description Admin-only endpoints for runtime image-source management.
// @tag.name Health
// @tag.description Service health and probe endpoints.
func main() {
	// 通过命令行参数覆盖配置文件位置和运行环境，方便在不同部署环境下复用同一份启动程序。
	configFile := flag.String("config", "", "path to config file")
	configEnv := flag.String("env", "", "app environment override")
	flag.Parse()

	// 从 configs 目录加载验证码服务配置。
	// LoadCaptchaConfigWithOptions 会根据传入的配置文件和环境变量解析出完整的运行配置。
	cfg, err := config.LoadCaptchaConfigWithOptions(config.LoadOptions{
		ConfigPath: "configs",
		ConfigFile: *configFile,
		Env:        *configEnv,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	// 使用彩色 console 日志，方便本地和终端里快速区分日志级别。
	logger, err := logging.NewColorLogger()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	// 打印关键启动参数，便于排查端口、注册中心开关等部署问题。
	logger.Info("starting captcha service",
		zap.Int("http_port", cfg.HTTP.Port),
		zap.Int("grpc_port", cfg.Grpc.Port),
		zap.Bool("nacos_enabled", cfg.Nacos.Enable))

	// 初始化 Redis 客户端。
	// 验证码业务、token 业务和健康检查都依赖 Redis，因此这里在启动阶段就建立连接。
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

	// 用一个带超时的上下文做连通性检查，避免 Redis 不可用时服务“假启动”。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		logger.Fatal("failed to connect redis", zap.Error(err))
	}
	logger.Info("redis connected")

	// 创建验证码服务，负责验证码图片、校验逻辑以及运行时图片源管理。
	captchaComponents := captchabootstrap.NewCaptchaComponents(rdb, &cfg.Captcha, logger)
	imageSourceComponents, err := captchabootstrap.NewRuntimeImageSourceComponents(rdb, &cfg.Captcha, captchaComponents.ImagePool, logger)
	if err != nil {
		logger.Fatal("failed to enable runtime image source manager", zap.Error(err))
	}
	captchaUseCase := captchaComponents.Captcha
	tokenUseCase := captchabootstrap.NewTokenUseCase(rdb, &cfg.Token)
	imageSourceUseCase := imageSourceComponents.UseCase
	// gRPC 暴露的服务实现基于 token use case，提供给外部系统直接调用。
	grpcService := grpcadapter.NewCaptchaTokenService(tokenUseCase)

	// 如果配置启用了图片池，则启动后台刷新任务，定期预热可用图片资源。
	if cfg.Captcha.ImagePool.Enabled {
		logger.Info("starting image pool refresh job")
		if err := captchaComponents.Lifecycle.StartImageRefresh(context.Background()); err != nil {
			logger.Error("failed to start image pool refresh job", zap.Error(err))
		}
	}

	// HTTP 控制器层：负责对外提供验证码接口、运行时图片源管理接口以及健康检查接口。
	httpHandler := &httptransport.CaptchaHandler{
		Captcha: captchaUseCase,
		Token:   tokenUseCase,
		Logger:  logger,
	}
	// 图片源管理接口通常只给管理员调用，因此单独放在一个 handler 中。
	imageSourceHandler := &httptransport.ImageSourceAdminHandler{
		ImageSource: imageSourceUseCase,
		Logger:      logger,
	}

	// 健康检查依赖 Redis，用于给 k8s/负载均衡/运维系统提供探针结果。
	healthHandler := httptransport.NewHealthHandler(rdb, logger)
	// 路由组装在这里集中完成，统一挂载鉴权中间件。
	router := httptransport.NewCaptchaRouter(
		httpHandler,
		imageSourceHandler,
		healthHandler,
		httptransport.NewAdminAuthMiddleware(logger, httptransport.RoleAdmin),
	)

	// HTTP 服务用于承载 REST 接口，超时参数用于避免慢连接占用资源过久。
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// gRPC 服务通过单独端口提供，便于内部系统以高效方式调用 token 接口。
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Grpc.Port))
	if err != nil {
		logger.Fatal("failed to listen grpc", zap.Error(err))
	}

	// 注册 unary 拦截器，用于统一记录每个 RPC 的耗时和错误。
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(UnaryLoggerInterceptor(logger)))
	captchapb.RegisterCaptchaTokenServiceServer(grpcServer, grpcService)
	// 打开反射，方便调试和联调用 grpcurl、BloomRPC 之类的工具。
	reflection.Register(grpcServer)

	// 将当前实例注册到 Nacos，供服务发现、流量转发和健康检查使用。
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

	// HTTP 和 gRPC 都在独立 goroutine 中启动，避免阻塞主协程。
	go func() {
		logger.Info("captcha http server listening", zap.Int("port", cfg.HTTP.Port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server exited unexpectedly", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("captcha grpc server listening", zap.Int("port", cfg.Grpc.Port))
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Error("grpc server exited unexpectedly", zap.Error(err))
		}
	}()

	// 启动后稍等片刻，尽量确保监听端口已经就绪，再进行注册中心注册。
	time.Sleep(2 * time.Second)
	if err := nacosRegistry.Register(); err != nil {
		logger.Error("failed to register nacos service", zap.Error(err))
	}

	// 主协程阻塞等待退出信号，收到 SIGINT/SIGTERM 后进入优雅关闭流程。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// 先停止后台刷新任务，避免服务退出时仍然继续访问外部资源。
	captchaComponents.Lifecycle.StopImageRefresh()

	// 先从注册中心摘除实例，避免后续流量继续打到即将关闭的节点。
	if err := nacosRegistry.Deregister(); err != nil {
		logger.Error("failed to deregister nacos service", zap.Error(err))
	}

	// 给注册中心和下游一些缓冲时间，减少关闭过程中出现的竞态。
	time.Sleep(1 * time.Second)

	// HTTP 服务使用带超时的 Shutdown，等待正在处理的请求完成。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown http server", zap.Error(err))
	} else {
		logger.Info("http server closed")
	}

	// gRPC 服务使用 GracefulStop，等待现有 RPC 完成后再关闭连接。
	grpcServer.GracefulStop()
	logger.Info("grpc server closed")
	logger.Info("captcha service stopped")
}

func UnaryLoggerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 在调用前后记录时间，用于统计每个 RPC 的执行耗时。
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// 统一补充方法名和耗时字段，方便在日志系统里聚合和检索。
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		// 失败时记录错误日志，成功时记录普通访问日志。
		if err != nil {
			logger.Error("rpc request failed", append(fields, zap.Error(err))...)
			return resp, err
		}

		logger.Info("rpc request succeeded", fields...)
		return resp, nil
	}
}
