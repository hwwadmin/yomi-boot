# quick-starter-demo

yomi-boot 组件库的快速启动示例：只有一个 `main` 函数和一个 `config` 目录，**没有任何业务**。

用途是验证组件库本身能否跑通：配置加载与校验 → 内置插件链初始化 → 默认中间件装配 → HTTP 监听 → 就绪钩子 → 优雅停机。

`config/dev.yml` 覆盖了全部内置插件配置段，仅排除分布式微服务组件 `consul` 与 `dtm`（保持 `enabled: false`）。其余插件（db / redis / kafka / redis_stream / oss / idempotency / token / http_record / event / pprof / ratelimit 等）全部启用。

