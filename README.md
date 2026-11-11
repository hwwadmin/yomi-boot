# yomi-boot

Go 基础设施库（能力库 + 插件生命周期），不含业务规则。  

当前底座已足够支撑绝大多数企业业务。

默认按需裁剪（改 YAML `enabled` 等开关即可）。


## 中间件推荐适配版本

以下为业务工程对接本库时的**运行侧基础设施推荐适配版本**（非 Go 客户端库版本）。按需启用，未用到的中间件无需部署。Docker 镜像 tag 均为已发布的具体版本，不要用 `latest`。

| 中间件 | 推荐适配版本 | Docker 镜像 |
|--------|--------------|-------------|
| MySQL | **8.4 LTS**（`8.4.11`） | `mysql:8.4.11` |
| Redis | **7.4**（`7.4.11`） | `redis:7.4.11` |
| Kafka | **Apache Kafka 4.2.2** | `apache/kafka:4.2.2`（Confluent 版 `confluentinc/cp-kafka:8.2.4`） |
| PostgreSQL | **16.15** | `postgres:16-alpine` |
| RustFS | **1.0.1** | `rustfs/rustfs:1.0.1` |

> 分布式能力见 [yomi-boot-cloud](https://github.com/hwwadmin/yomi-boot-cloud)。

## 快速启动

业务入口可直接使用根包 `boot` 完成基础设施初始化、路由装配、HTTP 服务构建与优雅停机：

```go
package main

import (
	"github.com/hwwadmin/yomi-boot/boot"
	"github.com/hwwadmin/yomi-boot/component/core/router"
	"github.com/hwwadmin/yomi-boot/plugin"
)

func main() {
	boot.Run(wire)
}

// wire 是路由装配入口；示例无业务，返回空注册器列表。
func wire(*plugin.PluginSlot) []router.Registrar {
	return nil
}
```

## 许可证

本项目采用 [Apache License 2.0](./LICENSE) 开源许可。
