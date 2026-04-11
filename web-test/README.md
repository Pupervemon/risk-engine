# 验证码调试说明

`web-test` 目录提供两类调试入口：

- `index.html`: 浏览器端滑块验证码调试页
- `test_grpc_client.go`: 验证码 Token 的 gRPC 校验工具

## 启动后端

在项目根目录执行：

```bash
go run ./cmd/captcha-server --env dev
```

也可以显式指定配置文件：

```bash
go run ./cmd/captcha-server --config ./configs/captcha.dev.yaml
```

默认 HTTP 地址是 `http://localhost:8091`，默认 gRPC 地址是 `localhost:9091`。

## 浏览器调试页

直接打开 `web-test/index.html` 即可，不需要额外的静态文件服务。

页面默认请求：

```text
http://localhost:8091/api/v1
```

后端地址支持两种覆盖方式：

1. 在页面顶部输入框中填写 API Base 并保存，值会写入 `localStorage`
2. 在 URL 上追加查询参数，例如：

```text
web-test/index.html?api=http://localhost:8091/api/v1
```

## gRPC 校验工具

```bash
go run ./web-test/test_grpc_client.go -addr localhost:9091 <TOKEN>
```

如果不传 `-addr`，默认连接 `localhost:9091`。

## 常用接口

- 获取验证码: `GET http://localhost:8091/api/v1/captcha`
- 校验验证码: `POST http://localhost:8091/api/v1/captcha/verify`
- 健康检查: `GET http://localhost:8091/health`

## 参考文档

- [配置契约](../docs/CONFIG_GUIDE.md)
- [项目总览](../README.md)
