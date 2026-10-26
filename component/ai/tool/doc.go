// Package tool 提供协议无关的工具定义、注册与安全执行门面。
//
// 本包只负责“工具”这一半：工具怎么描述、怎么注册、怎么按名字执行、结果与错误怎么
// 交给上层。它不调用大模型、不拼提示词、不做 Agent 循环，也不绑定任何 provider 协议；
// 模型侧的工具绑定与消息组装由调用方或后续能力包完成。
//
// # 基本用法
//
//	type weatherArgs struct {
//	    City string `json:"city"`
//	}
//
//	type weatherResult struct {
//	    City    string `json:"city"`
//	    Weather string `json:"weather"`
//	}
//
//	weather, err := tool.NewTyped(tool.Spec{
//	    Name:        "get_weather",
//	    Description: "查询指定城市的天气",
//	    Parameters: json.RawMessage(`{
//	        "type": "object",
//	        "properties": {
//	            "city": {"type": "string", "description": "城市名"}
//	        },
//	        "required": ["city"]
//	    }`),
//	    Metadata: map[string]string{"risk": "read"},
//	}, func(ctx context.Context, in weatherArgs) (weatherResult, error) {
//	    return queryWeather(ctx, in.City)
//	})
//	if err != nil {
//	    return err
//	}
//
//	registry, err := tool.New(tool.Config{Tools: []tool.Tool{weather}})
//	if err != nil {
//	    return err
//	}
//
//	// Specs 交给模型绑定；Invoke 执行模型返回的调用
//	specs := registry.Specs()
//	result, err := registry.Invoke(ctx, tool.Call{
//	    ID:        "call-1",
//	    Name:      "get_weather",
//	    Arguments: json.RawMessage(`{"city":"北京"}`),
//	})
//
// # 契约
//
//   - Spec.Parameters 是根类型为 object 的原生 JSON Schema；注册时只校验根节点形态、
//     type == object 与 required 为不重复的非空字符串数组，不解析 $ref/allOf，不校验
//     嵌套类型、枚举与范围。运行时参数只校验 JSON 对象与根级 required 键存在；类型、
//     嵌套与业务语义由工具实现负责
//   - Spec.Metadata 仅宿主可见，不发给模型；用于租户、风险等级、审计标签等，库只存不用
//   - Registry 构造后不可变，可并发调用；Specs 返回深拷贝，工具定义在注册时快照
//   - Spec.Name 必须是 ASCII 字母、数字、下划线或短横线，长度 1~64；注册时拒绝仅大小写
//     不同的重名，查找大小写敏感
//   - 工具实现须并发安全；实现自行持有资源，本包不提供 Close
//   - Invoke 把空参数或 null 归一为 {}，参数与结果都做防御性拷贝；结果必须是合法 JSON，
//     nil 等价于 null；参数或结果超限、结果非法都只让当前调用失败
//   - Invoke 不内置超时，使用调用方 ctx；工具实现必须尊重 ctx
//   - InvokeAll 按输入顺序返回 []Outcome；concurrency <= 1 顺序执行，大于 1 使用固定
//     worker 数；单点失败不影响兄弟调用，不重试，也不提供跨工具事务；调用期间调用方
//     不得修改 calls 或其 Arguments
//   - 业务应通过 Registry.Invoke 调用；直接调用 Tool.Invoke 时参数契约由调用方自行保证
//   - NewTyped 不生成 Schema，参数结构与 Schema 的同步由作者负责；解码沿用
//     encoding/json 默认行为，未知字段忽略
//   - ValidateSpec、ValidateArguments 与 DecodeArgs 是契约的可执行形式，供自定义 Tool
//     实现与直接调用方复用
//
// # 错误
//
// 对外错误统一为 *Error，含 Code、Message、Tool、CallID 四个安全字段。Error() 只返回
// 错误码与安全文案，不包含底层错误；Error 不实现 Unwrap，底层原因仅能通过 Cause 显式
// 获取，且只能用于进程内日志/告警，禁止写入 API 响应、消息队列、模型消息或前端。
//
// 错误码：
//
//	AI_TOOL_INVALID_ARGUMENT  定义、参数或配置非法
//	AI_TOOL_NOT_FOUND         工具未注册
//	AI_TOOL_EXECUTION         工具实现返回错误
//	AI_TOOL_TIMEOUT           调用超时
//	AI_TOOL_CANCELED          调用取消
//	AI_TOOL_PANIC             工具实现 panic
//	AI_TOOL_BAD_RESPONSE      返回内容非法或超限
//
// ErrInvalidArgument、ErrToolNotFound、ErrExecution、ErrPanic、ErrBadResponse 供
// errors.Is 使用；ErrExecution 是执行类总哨兵，覆盖执行错误、超时、取消、panic 与
// 非法输出。需要精确区分时读取 Error.Code。
//
// 工具实现可让错误实现 SafeError 提供对外安全文案；库会优先使用该文案，底层错误仍只进
// Cause。超时与取消可通过 Error.Code 或 errors.Is(err, context.Canceled/
// context.DeadlineExceeded) 判断，Cause 仍保留原始原因。
//
// # 高可用默认
//
// 参数与结果默认上限均为 8 MiB，可通过 Config.MaxArgumentsBytes / MaxContentBytes
// 调整；0 表示默认值，负数非法。超限返回安全错误而不是截断，避免把半截 JSON 交给模型。
// 上限是进入模型前的闸门，不能阻止工具自己先分配大对象，工具实现仍要自控资源。
//
// 批量默认顺序执行，调用方显式提高并发；worker 数固定，不会按调用数起协程。工具 panic
// 与批量 worker panic 都会被恢复为 CodePanic，单个调用失败不会拖垮整批。InvokeAll
// 共享传入 ctx；需要单调用独立超时时，调用方自行用 Invoke 派生 per-call ctx。
//
// # 安全
//
// 工具参数来自模型输出，必须按不可信输入处理；注册表成员即白名单，但工具名不是权限边界，
// 授权、人工审批与副作用控制由业务在 Registry 或 Tool 包装层完成。工具是进程内受信代码，
// 执行不可信代码需要独立沙箱。本包不打日志、不做脱敏、不做限流；审计与脱敏由调用方包装
// Tool 或记录 Cause 时自行处理。
//
// 本包与 ai 其他组件零依赖，不读 YAML、不进插件链、不做限流、不管理资源生命周期。
//
// @author zwei
package tool
