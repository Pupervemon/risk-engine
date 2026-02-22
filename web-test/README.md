# 验证码服务前端测试指南

## 快速开始

### 1. 启动后端服务

在项目根目录执行：

**Windows:**
```powershell
.\start.ps1
```

**Linux/Mac:**
```bash
./start.sh
```

### 2. 打开测试页面

直接在浏览器中打开 `web-test/index.html`，无需额外的web服务器。

## 功能说明

### ✅ 已实现功能

1. **滑块验证码生成**
   - 自动从后端获取验证码图片
   - 显示背景图和滑块图片

2. **鼠标轨迹追踪**
   - 记录滑动过程中的所有鼠标位置
   - 记录每个点的时间戳（毫秒级）
   - 只在后端要求时才发送轨迹数据

3. **智能验证**
   - 根据后端配置决定是否需要轨迹验证
   - 支持位置精度验证
   - 支持轨迹行为分析（防机器人）

4. **详细日志输出**
   - 在浏览器控制台查看完整的请求和响应数据
   - 显示验证失败的详细原因

## API 接口说明

### 获取验证码

**请求:**
```
GET http://localhost:8080/api/v1/captcha
```

**响应:**
```json
{
  "captchaId": "abc123...",
  "masterImage": "data:image/png;base64,...",
  "tileImage": "data:image/png;base64,...",
  "targetY": 60,
  "expiresIn": 120,
  "requireMouseTrack": true
}
```

### 验证验证码

**请求:**
```
POST http://localhost:8080/api/v1/captcha/verify
Content-Type: application/json

{
  "captchaId": "abc123...",
  "pointX": 150,
  "pointY": 0,
  "mouseTrack": [
    {"x": 0, "y": 0, "t": 0},
    {"x": 10, "y": 0, "t": 50},
    {"x": 150, "y": 0, "t": 500}
  ]
}
```

**成功响应:**
```json
{
  "token": "eyJhbGc...",
  "expiresIn": 1800
}
```

**失败响应:**
```json
{
  "error": "CAPTCHA_INVALID",
  "reason": "POSITION_MISMATCH"
}
```

## 测试技巧

### 查看详细信息

按 F12 打开浏览器开发者工具，切换到 Console 标签，可以看到：

- 验证码加载信息
- 鼠标轨迹点数和持续时间
- 验证请求的完整数据
- 后端返回的token信息

### 测试不同场景

1. **正常滑动** - 慢速拖动滑块到正确位置
2. **快速滑动** - 快速拖动测试轨迹验证
3. **错误位置** - 故意滑到错误位置查看反馈
4. **多次失败** - 测试失败后自动刷新功能

## 配置修改

### 修改后端地址

编辑 `index.html` 第 95 行：
```javascript
const API_BASE = 'http://localhost:8080/api/v1';
```

### 后端配置

后端配置文件在 `configs/captcha.dev.yaml`，可以调整：

- `track_validation.enabled` - 是否启用轨迹验证
- `track_validation.min_points` - 最少轨迹点数
- `ttl_seconds` - 验证码有效期
- `tolerance` - 位置容差

## 常见问题

### 1. 无法连接后端

**症状:** 页面显示 "获取失败，请确保后端 8080 端口已启动"

**解决方法:**
- 确认后端服务已启动
- 检查端口是否被占用
- 查看后端日志确认服务状态

### 2. 验证总是失败

**症状:** 滑到正确位置仍然失败

**解决方法:**
- 查看控制台日志中的 `reason` 字段
- 如果是 `TRACK_SUSPICIOUS`，尝试更自然地滑动
- 如果是 `POSITION_MISMATCH`，微调位置容差配置

### 3. CORS 错误

**症状:** 浏览器控制台显示 CORS 跨域错误

**解决方法:**
- 确认后端已更新（包含CORS中间件）
- 重新编译并启动后端服务

## 技术细节

### 鼠标轨迹数据格式

```typescript
interface TrackPoint {
  x: number;    // X坐标（相对于容器）
  y: number;    // Y坐标（始终为0，单轴滑动）
  t: number;    // 时间戳（毫秒，相对于开始时间）
}
```

### 轨迹验证算法

后端会检查：

1. **轨迹点数量** - 太少说明可能是脚本
2. **时间合理性** - 太快或太慢都可疑
3. **速度变化** - 人类滑动会有加速减速
4. **连续性** - 位置跳变说明可能是模拟
5. **方向性** - 是否有明显的回退或震荡

## 下一步

查看完整的文档：
- [配置指南](../docs/CONFIG_GUIDE.md)
- [前端集成指南](../docs/FRONTEND_INTEGRATION.md)
- [快速参考](../docs/QUICK_REFERENCE.md)
