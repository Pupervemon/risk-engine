package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// RiskConfig Risk服务总配置
type RiskConfig struct {
	HTTP      HTTPConfig      `mapstructure:"http"`       // 引用 shared_config.go
	Grpc      GrpcConfig      `mapstructure:"grpc"`       // 引用 shared_config.go
	Redis     RedisConfig     `mapstructure:"redis"`      // 引用 shared_config.go
	Nacos     NacosConfig     `mapstructure:"nacos"`      // 引用 shared_config.go
	RiskRules RiskRulesConfig `mapstructure:"risk_rules"` // Risk 独有
}

// RiskRulesConfig 风控规则配置
type RiskRulesConfig struct {
	Login         LoginRuleConfig     `mapstructure:"login"`
	IpRateLimit   IPRateLimitConfig   `mapstructure:"ip_rate_limit"`
	UserRateLimit UserRateLimitConfig `mapstructure:"user_rate_limit"`
}

type LoginRuleConfig struct {
	MaxFailCount           int `mapstructure:"max_fail_count"`
	FailCountExpireMinutes int `mapstructure:"fail_count_expire_minutes"`
}

type IPRateLimitConfig struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

type UserRateLimitConfig struct {
	OnlineSelfTest  UserRateLimitRule `mapstructure:"online_self_test"`
	JudgeSubmission UserRateLimitRule `mapstructure:"judge_submission"`
}

type UserRateLimitRule struct {
	Limit         int `mapstructure:"limit"`
	WindowSeconds int `mapstructure:"window_seconds"`
}

// Risk 加载逻辑

// LoadRiskConfig 加载 Risk 服务配置
func LoadRiskConfig(configPath string) (*RiskConfig, error) {
	// [1] 加载 .env
	_ = godotenv.Load()

	// [2] 确定环境
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENV")
	}
	if env == "" {
		env = "prod" // 默认生产环境
	}

	fmt.Printf("[RiskConfig] 正在加载环境: %s\n", env)

	v := viper.New()

	// [3] 开启自动环境变量映射
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// [4] 配置文件定位
	// 文件名格式: risk.dev.yaml 或 risk.prod.yaml
	configName := fmt.Sprintf("risk.%s", env)
	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	// 添加搜索路径
	v.AddConfigPath(configPath)
	v.AddConfigPath("./configs")
	v.AddConfigPath("../configs")
	v.AddConfigPath(".")

	// [5] 读取配置
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s.yaml 失败: %w", configName, err)
	}
	fmt.Printf("[RiskConfig] 使用配置文件: %s\n", v.ConfigFileUsed())

	// [6] 解析到结构体
	var cfg RiskConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置结构失败: %w", err)
	}

	if cfg.Nacos.Enable {
		if cfg.Nacos.Metadata == nil {
			cfg.Nacos.Metadata = make(map[string]string)
		}
		// 1. 自动填入 gRPC 端口 (解决 Java 找不到服务的问题)
		cfg.Nacos.Metadata["gRPC_port"] = strconv.Itoa(cfg.Grpc.Port)
	}
	// [7] 校验配置
	if err := cfg.Validate(env); err != nil {
		return nil, err
	}

	// [8] 打印摘要
	cfg.Print()

	return &cfg, nil
}

//  辅助方法

// Validate 校验 Risk 配置
func (c *RiskConfig) Validate(env string) error {
	if c.Redis.Addr == "" {
		return fmt.Errorf("Redis地址不能为空")
	}
	if c.HTTP.Port <= 0 {
		return fmt.Errorf("HTTP端口无效")
	}

	// 生产环境严格检查
	if env == "prod" {
		if c.Redis.Password == "" {
			return fmt.Errorf("[安全阻断] 生产环境 Redis 密码不能为空")
		}
	}
	return nil
}

// Print 打印 Risk 配置摘要
func (c *RiskConfig) Print() {
	fmt.Println("\n=========== Risk 服务配置 ===========")
	fmt.Printf("HTTP端口: %d\n", c.HTTP.Port)
	fmt.Printf("gRPC端口: %d\n", c.Grpc.Port)

	fmt.Println("-------------------------------------")
	fmt.Printf("Redis地址: %s\n", c.Redis.Addr)
	fmt.Printf("Redis DB : %d\n", c.Redis.DB)
	// 简单脱敏
	passMask := "<无>"
	if len(c.Redis.Password) > 0 {
		passMask = "******"
	}
	fmt.Printf("Redis密码: %s\n", passMask)

	fmt.Println("-------------------------------------")
	if c.Nacos.Enable {
		ns := c.Nacos.Namespace
		if ns == "" {
			ns = "public (默认)"
		}
		fmt.Printf("Nacos: 启用\n")
		fmt.Printf("地址 : %s\n", c.Nacos.ServerAddr)
		fmt.Printf("空间 : %s\n", ns)
		fmt.Printf("元数据: %v\n", c.Nacos.Metadata)
	} else {
		fmt.Printf("Nacos: [禁用]\n")
	}

	fmt.Println("-------------------------------------")
	fmt.Printf("风控规则-IP限流: %d次/%d秒\n",
		c.RiskRules.IpRateLimit.Limit, c.RiskRules.IpRateLimit.WindowSeconds)
	fmt.Printf("风控规则-登录失败: %d次 (锁定%d分钟)\n",
		c.RiskRules.Login.MaxFailCount, c.RiskRules.Login.FailCountExpireMinutes)

	fmt.Println("=====================================\n")
}
