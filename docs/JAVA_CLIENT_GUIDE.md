# Java 客户端调用 Risk Engine gRPC 服务指南

## 问题诊断：HTTP/2 Exception

当 Java 客户端调用 Risk Engine 时出现 HTTP/2 异常，通常是以下原因：

### 常见错误信息
```
io.grpc.StatusRuntimeException: UNAVAILABLE: io exception
Caused by: io.netty.handler.codec.http2.Http2Exception
```

### 原因分析

1. **连接类型不匹配**：服务端使用明文连接（plaintext），客户端尝试 TLS 连接
2. **端口配置错误**：
   - gRPC 端口：`9090`（业务接口）
   - HTTP 端口：`9080`（健康检查，不是 gRPC）
3. **网络/防火墙问题**
4. **依赖版本不兼容**

## 解决方案

### 1. 正确的 Java gRPC 客户端配置

#### Maven 依赖
```xml
<dependencies>
    <!-- gRPC Core -->
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-netty-shaded</artifactId>
        <version>1.60.0</version>
    </dependency>
    
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-protobuf</artifactId>
        <version>1.60.0</version>
    </dependency>
    
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-stub</artifactId>
        <version>1.60.0</version>
    </dependency>
</dependencies>
```

#### Gradle 依赖
```gradle
dependencies {
    implementation 'io.grpc:grpc-netty-shaded:1.60.0'
    implementation 'io.grpc:grpc-protobuf:1.60.0'
    implementation 'io.grpc:grpc-stub:1.60.0'
}
```

### 2. 创建 gRPC Channel（明文连接）

#### 方式一：直连（开发环境）
```java
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;

public class RiskServiceClient {
    
    private final ManagedChannel channel;
    private final RiskControlServiceGrpc.RiskControlServiceBlockingStub blockingStub;
    
    public RiskServiceClient(String host, int port) {
        // 重要：使用 usePlaintext() 表示明文连接
        this.channel = ManagedChannelBuilder
            .forAddress(host, port)
            .usePlaintext()  // 关键配置！！！
            .build();
        
        this.blockingStub = RiskControlServiceGrpc.newBlockingStub(channel);
    }
    
    public CheckResponse check(CheckRequest request) {
        return blockingStub.check(request);
    }
    
    public void shutdown() throws InterruptedException {
        channel.shutdown().awaitTermination(5, TimeUnit.SECONDS);
    }
}

// 使用示例
public class Main {
    public static void main(String[] args) {
        // 连接到 Risk Engine gRPC 服务
        RiskServiceClient client = new RiskServiceClient("118.24.164.222", 9090);
        
        try {
            CheckRequest request = CheckRequest.newBuilder()
                .setReqId(UUID.randomUUID().toString())
                .setIp("192.168.1.100")
                .setUserId("user123")
                .setScene(Scene.SCENE_LOGIN)
                .build();
            
            CheckResponse response = client.check(request);
            System.out.println("Action: " + response.getAction());
            System.out.println("Reason: " + response.getReason());
            
        } finally {
            client.shutdown();
        }
    }
}
```

#### 方式二：通过 Nacos 服务发现（生产环境推荐）
```java
import com.alibaba.nacos.api.NacosFactory;
import com.alibaba.nacos.api.naming.NamingService;
import com.alibaba.nacos.api.naming.pojo.Instance;

public class RiskServiceClientWithNacos {
    
    private final NamingService namingService;
    private final String serviceName = "risk-service";
    
    public RiskServiceClientWithNacos(String nacosAddr) throws Exception {
        Properties properties = new Properties();
        properties.put("serverAddr", nacosAddr);
        this.namingService = NacosFactory.createNamingService(properties);
    }
    
    public ManagedChannel createChannel() throws Exception {
        // 从 Nacos 获取服务实例
        Instance instance = namingService.selectOneHealthyInstance(serviceName);
        
        // 从元数据中获取 gRPC 端口
        String grpcPort = instance.getMetadata().get("grpc-port");
        int port = grpcPort != null ? Integer.parseInt(grpcPort) : 9090;
        
        return ManagedChannelBuilder
            .forAddress(instance.getIp(), port)
            .usePlaintext()
            .build();
    }
}
```

### 3. Spring Boot 集成

#### application.yml 配置
```yaml
grpc:
  client:
    risk-service:
      address: 'static://118.24.164.222:9090'  # 直连
      # address: 'nacos://risk-service'        # 通过 Nacos
      negotiation-type: plaintext              # 明文连接
      
# 或使用 Nacos 服务发现
spring:
  cloud:
    nacos:
      discovery:
        server-addr: 118.24.164.222:8848
```

#### Java 配置类
```java
import net.devh.boot.grpc.client.inject.GrpcClient;
import org.springframework.stereotype.Service;

@Service
public class RiskService {
    
    @GrpcClient("risk-service")
    private RiskControlServiceGrpc.RiskControlServiceBlockingStub riskStub;
    
    public boolean checkRisk(String userId, String ip) {
        CheckRequest request = CheckRequest.newBuilder()
            .setReqId(UUID.randomUUID().toString())
            .setIp(ip)
            .setUserId(userId)
            .setScene(Scene.SCENE_LOGIN)
            .build();
        
        CheckResponse response = riskStub.check(request);
        return response.getAction() == Action.ACTION_PASS;
    }
}
```

## 4. 连接配置对比

| 配置项 | 开发环境 | 生产环境 |
|--------|----------|----------|
| **服务器地址** | 127.0.0.1 | 118.24.164.222 |
| **gRPC 端口** | 9090 | 9090 |
| **连接类型** | plaintext | plaintext（或 TLS） |
| **服务发现** | 直连 | Nacos |
| **超时设置** | 5s | 3s |

## 5. 验证连接

### 测试代码
```java
public class ConnectionTest {
    public static void main(String[] args) {
        System.out.println("测试 Risk Service gRPC 连接...");
        
        ManagedChannel channel = ManagedChannelBuilder
            .forAddress("118.24.164.222", 9090)
            .usePlaintext()
            .build();
        
        try {
            RiskControlServiceGrpc.RiskControlServiceBlockingStub stub = 
                RiskControlServiceGrpc.newBlockingStub(channel);
            
            CheckRequest request = CheckRequest.newBuilder()
                .setReqId("test-" + System.currentTimeMillis())
                .setIp("test-ip")
                .setUserId("test-user")
                .setScene(Scene.SCENE_LOGIN)
                .build();
            
            CheckResponse response = stub.check(request);
            System.out.println("✅ 连接成功！");
            System.out.println("响应: " + response);
            
        } catch (Exception e) {
            System.err.println("❌ 连接失败: " + e.getMessage());
            e.printStackTrace();
        } finally {
            channel.shutdown();
        }
    }
}
```

## 6. 常见错误排查

### 错误 1: UNAVAILABLE: io exception
**原因**: 没有使用 `usePlaintext()`
```java
// ❌ 错误
ManagedChannelBuilder.forAddress(host, port).build();

// ✅ 正确
ManagedChannelBuilder.forAddress(host, port).usePlaintext().build();
```

### 错误 2: 连接超时
**原因**: 端口或地址错误
```java
// 检查配置
String host = "118.24.164.222";  // ✅ 正确的服务器地址
int port = 9090;                  // ✅ gRPC 端口（不是 9080）
```

### 错误 3: SSL Handshake 失败
**原因**: 服务端是明文，客户端尝试 TLS
```java
// ✅ 明文连接
.usePlaintext()

// 如果服务端启用了 TLS（未来）
// .useTransportSecurity()
```

## 7. 性能优化建议

### 连接池配置
```java
public class RiskServiceClientPool {
    private final ManagedChannel channel;
    
    public RiskServiceClientPool(String host, int port) {
        this.channel = ManagedChannelBuilder
            .forAddress(host, port)
            .usePlaintext()
            .keepAliveTime(60, TimeUnit.SECONDS)        // 保持连接
            .keepAliveTimeout(20, TimeUnit.SECONDS)     // 超时时间
            .maxInboundMessageSize(10 * 1024 * 1024)   // 最大消息 10MB
            .build();
    }
    
    public RiskControlServiceGrpc.RiskControlServiceBlockingStub getStub() {
        return RiskControlServiceGrpc.newBlockingStub(channel)
            .withDeadlineAfter(3, TimeUnit.SECONDS);  // 设置请求超时
    }
}
```

## 8. 完整示例项目结构

```
src/
├── main/
│   ├── java/
│   │   └── com/
│   │       └── example/
│   │           ├── client/
│   │           │   ├── RiskServiceClient.java
│   │           │   └── RiskServiceClientPool.java
│   │           ├── config/
│   │           │   └── GrpcClientConfig.java
│   │           └── service/
│   │               └── LoginService.java
│   └── resources/
│       └── application.yml
└── proto/
    └── risk.proto  (从 risk-proto 仓库复制)
```

## 9. 调试技巧

### 启用 gRPC 调试日志
```java
// 方式一：Java 系统属性
System.setProperty("io.grpc.netty.NettyChannelBuilder.logLevel", "DEBUG");

// 方式二：Logback 配置
<logger name="io.grpc" level="DEBUG"/>
<logger name="io.netty" level="DEBUG"/>
```

### 使用 grpcurl 测试（推荐）
```bash
# 安装 grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# 列出服务
grpcurl -plaintext 118.24.164.222:9090 list

# 调用接口
grpcurl -plaintext \
  -d '{
    "req_id": "test123",
    "ip": "192.168.1.1",
    "user_id": "user123",
    "scene": "SCENE_LOGIN"
  }' \
  118.24.164.222:9090 \
  risk.v1.RiskControlService/Check
```

## 10. 联系方式

如果仍然遇到问题，请提供：
1. 完整的错误堆栈
2. Java gRPC 版本
3. 网络环境（内网/外网）
4. 是否使用 Spring Cloud Gateway
