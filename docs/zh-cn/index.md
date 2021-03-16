## 项目简介
&emsp;&emsp;&emsp;&emsp;
一个能够实现高性能 HTTP 转发、API 访问权限控制等目的的微服务网关，拥有强大的自定义插件系统可以自行扩展，提供友好的图形化配置界面，能够快速帮助企业进行 API 服务治理、提高 API 服务的稳定性和安全性。

### 注意
> 请确保你的Go版本是1.10或以上。否则,未定义的“math/rand”。编译时将发生Shuffle错误。

### 功能 
- 流量控制(Server或API级别)
- 熔断(Server或API级别)
- 负载均衡
- 服务发现
- 插件机制
- 路由(分流，复制流量)
- API 聚合
- API 参数校验
- API 访问控制（黑白名单）
- API 默认返回值
- API 定制返回值
- API 结果Cache
- JWT Authorization
- API Metric导入Prometheus
- API 失败重试
- 后端server的健康检查
- 开放管理API(GRPC、Restful)
- 支持 WebSocket(WSS)
- 支持 MQTT
- 支持在线迁移数据

### Docker

### 系统架构

### 目录结构
- config  配置文件
- filter  过滤器
- goetty  
- lbs     负载均衡
- log  日志  
- pb 
- proxy  代理
- router 路由表
- utils 常用功能
  * hack
  * uuid  唯一ID创建器
  * version  版本


### Web界面（WebUI）

