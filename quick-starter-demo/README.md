# quick-starter-demo

yomi-boot 组件库的快速启动示例：只有一个 `main` 函数和一个 `config` 目录，**没有任何业务**。

用途是验证组件库本身能否跑通：配置加载与校验 → 内置插件链初始化 → 默认中间件装配 → HTTP 监听 → 就绪钩子 → 优雅停机。

`config/dev.yml` 覆盖了全部内置插件配置段，仅排除分布式微服务组件 `consul` 与 `dtm`（保持 `enabled: false`）。其余插件（db / redis / kafka / redis_stream / oss / idempotency / token / http_record / event / pprof / ratelimit 等）全部启用。

## 本地依赖

| 中间件 | 地址 | 凭据 / 说明 |
|--------|------|-------------|
| MySQL | 127.0.0.1:3306 | `root` / `hww350521`，库 `quick-starter-demo` |
| Redis | 127.0.0.1:6379 | 密码 `hww350521` |
| Kafka | 127.0.0.1:9092 | SASL_PLAINTEXT + PLAIN，`zwei` / `hww350521` |
| RustFS | 127.0.0.1:9000 | `rustfsadmin` / `rustfsadmin`，桶 `quick-starter-demo` |

## 初始化

1. 创建数据库并导入表结构（幂等表与事件 outbox/inbox 表）：

   ```bash
   mysql -uroot -p -e 'CREATE DATABASE IF NOT EXISTS `quick-starter-demo` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;'
   mysql -uroot -p quick-starter-demo < ../sql/sys_biz.sql
   mysql -uroot -p quick-starter-demo < ../sql/sys_event.sql
   ```

2. 预建 Kafka 主题（客户端不会触发 broker 自动建 topic）：

   ```bash
   kafka-topics --create --if-not-exists --topic yomi.http.record --partitions 1 --replication-factor 1
   ```

3. （可选，验证 OSS）在 RustFS 中创建桶 `quick-starter-demo`。

## 运行

```bash
cd quick-starter-demo
go run .
```

默认监听 `:8080`，pprof 监听 `:16080`；环境由 `YOMI_CONFIG` 选择（local/dev/prod，默认 dev）。
