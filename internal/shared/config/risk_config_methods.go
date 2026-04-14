package config

import "fmt"

// Validate preserves the public method while reusing strict startup validation.
func (c *RiskConfig) Validate(env string) error {
	return validateRiskConfigStrict(c, env)
}

// Print outputs a redacted startup summary.
func (c *RiskConfig) Print() {
	fmt.Println()
	fmt.Println("=========== Risk Service Config ===========")
	fmt.Printf("HTTP port: %d\n", c.HTTP.Port)
	fmt.Printf("gRPC port: %d\n", c.Grpc.Port)

	fmt.Println("-------------------------------------------")
	fmt.Printf("Redis addr: %s\n", c.Redis.Addr)
	fmt.Printf("Redis DB: %d\n", c.Redis.DB)
	fmt.Printf("Redis password: %s\n", maskedConfigValue(c.Redis.Password))

	fmt.Println("-------------------------------------------")
	fmt.Printf("Nacos: %s\n", boolLabel(c.Nacos.Enable))
	if c.Nacos.Enable {
		fmt.Printf("Nacos server: %s\n", c.Nacos.ServerAddr)
		fmt.Printf("Nacos namespace: %s\n", displayNamespace(c.Nacos.Namespace))
		fmt.Printf("Nacos service: %s\n", c.Nacos.ServiceName)
		fmt.Printf("Nacos metadata: %v\n", c.Nacos.Metadata)
	}

	fmt.Println("-------------------------------------------")
	fmt.Printf("IP rate limit: %d / %ds\n",
		c.RiskRules.IpRateLimit.Limit, c.RiskRules.IpRateLimit.WindowSeconds)
	fmt.Printf("Login fail limit: %d (lock %d min)\n",
		c.RiskRules.Login.MaxFailCount, c.RiskRules.Login.FailCountExpireMinutes)
	fmt.Printf("User online_self_test: %d / %ds\n",
		c.RiskRules.UserRateLimit.OnlineSelfTest.Limit,
		c.RiskRules.UserRateLimit.OnlineSelfTest.WindowSeconds)
	fmt.Printf("User judge_submission: %d / %ds\n",
		c.RiskRules.UserRateLimit.JudgeSubmission.Limit,
		c.RiskRules.UserRateLimit.JudgeSubmission.WindowSeconds)

	fmt.Println("===========================================")
	fmt.Println()
}
