package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// LoadOptions controls how a service resolves config files and env overrides.
type LoadOptions struct {
	ConfigPath string
	ConfigFile string
	Env        string
}

type envBindings map[string][]string

func newServiceViper(serviceName string, servicePrefix string, options LoadOptions, sample any) (*viper.Viper, string, error) {
	_ = godotenv.Load()

	env := resolveAppEnv(servicePrefix, options.Env)
	configFile := resolveConfigFile(servicePrefix, options.ConfigFile)

	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindings := make(envBindings)
	collectStructBindings(bindings, reflect.TypeOf(sample), nil, servicePrefix)
	addCommonEnvAliases(bindings, servicePrefix)
	addLegacyEnvAliases(bindings, serviceName)

	if err := applyBindings(v, bindings); err != nil {
		return nil, "", err
	}

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName(fmt.Sprintf("%s.%s", serviceName, env))
		v.SetConfigType("yaml")
		if options.ConfigPath != "" {
			v.AddConfigPath(options.ConfigPath)
		}
		v.AddConfigPath("./configs")
		v.AddConfigPath("../configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if configFile != "" {
			return nil, "", fmt.Errorf("读取配置文件 %s 失败: %w", configFile, err)
		}
		return nil, "", fmt.Errorf("读取配置文件 %s.%s.yaml 失败: %w", serviceName, env, err)
	}

	return v, env, nil
}

func resolveAppEnv(servicePrefix string, explicitEnv string) string {
	if env := strings.TrimSpace(explicitEnv); env != "" {
		return env
	}
	if env := strings.TrimSpace(os.Getenv(servicePrefix + "_APP_ENV")); env != "" {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("APP_ENV")); env != "" {
		return env
	}
	return "prod"
}

func resolveConfigFile(servicePrefix string, explicitConfigFile string) string {
	if configFile := strings.TrimSpace(explicitConfigFile); configFile != "" {
		return configFile
	}
	if configFile := strings.TrimSpace(os.Getenv(servicePrefix + "_CONFIG_FILE")); configFile != "" {
		return configFile
	}
	if configFile := strings.TrimSpace(os.Getenv("CONFIG_FILE")); configFile != "" {
		return configFile
	}
	return ""
}

func collectStructBindings(bindings envBindings, t reflect.Type, path []string, servicePrefix string) {
	if t == nil {
		return
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		key := mapstructureKey(field)
		if key == "" || key == "-" {
			continue
		}

		nextPath := append(append([]string(nil), path...), key)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		switch fieldType.Kind() {
		case reflect.Struct:
			collectStructBindings(bindings, fieldType, nextPath, servicePrefix)
		case reflect.Map, reflect.Slice, reflect.Array:
			continue
		default:
			envParts := append([]string{servicePrefix}, nextPath...)
			addBinding(bindings, strings.Join(nextPath, "."), formatEnvName(envParts...))
		}
	}
}

func mapstructureKey(field reflect.StructField) string {
	tag := field.Tag.Get("mapstructure")
	if tag == "" {
		return ""
	}
	if idx := strings.Index(tag, ","); idx >= 0 {
		tag = tag[:idx]
	}
	return tag
}

func formatEnvName(parts ...string) string {
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, strings.ToUpper(part))
	}
	return strings.Join(segments, "_")
}

// addCommonEnvAliases 为 Nacos 和基础服务端口添加通用的环境变量别名。
func addCommonEnvAliases(bindings envBindings, servicePrefix string) {
	addBinding(bindings, "http.port", formatEnvName(servicePrefix, "server", "http", "port"))
	addBinding(bindings, "grpc.port", formatEnvName(servicePrefix, "server", "grpc", "port"))

	addBinding(bindings, "nacos.enable", formatEnvName(servicePrefix, "registry", "nacos", "enable"))
	addBinding(bindings, "nacos.server_addr", formatEnvName(servicePrefix, "registry", "nacos", "server_addr"))
	addBinding(bindings, "nacos.namespace", formatEnvName(servicePrefix, "registry", "nacos", "namespace"))
	addBinding(bindings, "nacos.service_name", formatEnvName(servicePrefix, "registry", "nacos", "service_name"))
	addBinding(bindings, "nacos.group_name", formatEnvName(servicePrefix, "registry", "nacos", "group_name"))
	addBinding(bindings, "nacos.cluster_name", formatEnvName(servicePrefix, "registry", "nacos", "cluster_name"))
	addBinding(bindings, "nacos.register_ip", formatEnvName(servicePrefix, "registry", "nacos", "register_ip"))
	addBinding(bindings, "nacos.weight", formatEnvName(servicePrefix, "registry", "nacos", "weight"))
}

// addLegacyEnvAliases 添加旧版的、扁平的环境变量绑定，确保平滑迁移。
func addLegacyEnvAliases(bindings envBindings, serviceName string) {
	addBinding(bindings, "redis.addr", "REDIS_ADDR")
	addBinding(bindings, "redis.password", "REDIS_PASSWORD")
	addBinding(bindings, "redis.db", "REDIS_DB")
	addBinding(bindings, "redis.pool_size", "REDIS_POOL_SIZE")
	addBinding(bindings, "redis.dial_timeout_seconds", "REDIS_DIAL_TIMEOUT_SECONDS")
	addBinding(bindings, "redis.read_timeout_seconds", "REDIS_READ_TIMEOUT_SECONDS")
	addBinding(bindings, "redis.write_timeout_seconds", "REDIS_WRITE_TIMEOUT_SECONDS")

	addBinding(bindings, "nacos.enable", "NACOS_ENABLE")
	addBinding(bindings, "nacos.server_addr", "NACOS_SERVER_ADDR")
	addBinding(bindings, "nacos.namespace", "NACOS_NAMESPACE")
	addBinding(bindings, "nacos.service_name", "NACOS_SERVICE_NAME")
	addBinding(bindings, "nacos.group_name", "NACOS_GROUP_NAME")
	addBinding(bindings, "nacos.cluster_name", "NACOS_CLUSTER_NAME")
	addBinding(bindings, "nacos.register_ip", "NACOS_REGISTER_IP")
	addBinding(bindings, "nacos.weight", "NACOS_WEIGHT")

	if serviceName == "captcha" {
		addBinding(bindings, "token.secret", "TOKEN_SECRET")
		addBinding(bindings, "captcha.external_image_api.url", "CAPTCHA_EXTERNAL_IMAGE_API_URL")
		addBinding(bindings, "captcha.external_image_api.api_key", "CAPTCHA_EXTERNAL_IMAGE_API_API_KEY")
	}
}

// addBinding 安全地向绑定映射中添加一个新的环境变量规则。
func addBinding(bindings envBindings, key string, envName string) {
	if key == "" || envName == "" {
		return
	}

	for _, existing := range bindings[key] {
		if existing == envName {
			return
		}
	}

	bindings[key] = append(bindings[key], envName)
}

// applyBindings 将收集到的所有绑定规则应用到 viper 实例。
func applyBindings(v *viper.Viper, bindings envBindings) error {
	for key, envNames := range bindings {
		args := append([]string{key}, envNames...)
		if err := v.BindEnv(args...); err != nil {
			return fmt.Errorf("绑定环境变量 %s 失败: %w", key, err)
		}
	}
	return nil
}

