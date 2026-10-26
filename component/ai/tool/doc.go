// Package tool 提供协议无关的工具定义、注册、安全执行与填槽能力。
//
// 一个粗粒度 tool 可以就是一个动作：模型按工具 Schema 生成入参 JSON，本包负责拼填槽
// 提示词、调用注入的模型端口、抽取 JSON、按 typed 契约校验、返回缺槽信息，再执行工具。
// 本包不新增 Action 概念，不选择工具，不做多工具编排，也不依赖具体模型或 provider。
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
//	    Fill: tool.FillSpec{
//	        Required:       []string{"city"},
//	        IdentifierKeys: []string{"city"},
//	        Guidance:       "city 必须从用户原文或历史中逐字提取",
//	        MissingPrompt:  "请提供城市名。",
//	    },
//	}, func(ctx context.Context, in weatherArgs) (weatherResult, error) {
//	    return queryWeather(ctx, in.City)
//	})
//	if err != nil {
//	    return err
//	}
//
//	registry, err := tool.New(tool.Config{
//	    Tools:     []tool.Tool{weather},
//	    Completer: completer, // 业务把 llm.Client 适配成 tool.Completer
//	})
//	if err != nil {
//	    return err
//	}
//
//	filled, err := registry.Fill(ctx, tool.Call{
//	    ID:       "call-1",
//	    Name:     "get_weather",
//	    Evidence: &tool.Evidence{UserInput: "北京今天天气"},
//	})
//	if err != nil {
//	    return err
//	}
//	if !filled.Complete {
//	    // filled.Missing / filled.MissingPrompt 交给业务追问用户
//	    return ask(filled.MissingPrompt, filled.Missing)
//	}
//	result, err := registry.InvokeFilled(ctx, filled)
//
// # 填槽
//
// Fill 按工具定义拼装 system/user 提示词，交给 Config.Completer：
//
//   - Schema：Spec.Parameters 原样进入提示词，是模型侧契约；
//   - IdentifierKeys：值必须能在 Evidence 原文中找到，防止模型臆造；
//   - RequiredKeys：Fill.Required 作为候选必填进入提示词，真正缺失仍由 Validate 判；
//   - Guidance/Example：动作专属填法，Example 只示范字段形状；
//   - Evidence：用户原文、历史、OCR 以 JSON 编码注入，避免破坏提示词结构；
//   - 缺槽不重试，返回 Filled.Missing；非法 JSON/类型错误可按 MaxAttempts 重试；
//   - Fill.EvidenceOnly 为 true 时跳过模型，要求 NewResolved 自定义 Decode 从证据映射；
//   - IdentifierKeys 非空或 EvidenceOnly 为 true 时 Fill 要求 Evidence 非 nil；Evidence
//     为 nil 时 Invoke 跳过门闩（程序化调用），对话填槽必须传 Evidence。
//
// 模型 JSON 会经过工具入参契约：
//
//   - simple tool 直接检查 JSON 对象、运行时必填与原始标识符；
//   - NewResolved 工具通过 InputContract 做 Decode/Normalize/Validate/Identify，
//     得到 typed 入参后再执行。
//
// 关键区别：Schema.required 是给模型的建议，运行时必填由 Fill.Required（无自定义
// Validate 时）或 InputContract.Validate 决定。二选一/条件必填只能由 Validate 表达，
// 不要为了省事把 Schema.required 写死。simple tool 声明了 Fill 规则时，Invoke 缺槽
// 返回 ErrInputIncomplete；未声明 Fill 的 simple tool 仍按 Schema required 返回
// ErrInvalidArgument。
//
// # 模型端口
//
// 本包不 import llm。业务把 llm.Client 适配成 Completer：
//
//	type llmCompleter struct{ client llm.Client }
//
//	func (c llmCompleter) Complete(ctx context.Context, req tool.CompletionRequest) (*tool.CompletionResponse, error) {
//	    resp, err := c.client.Chat(ctx, llm.ChatRequest{
//	        Messages: []llm.Message{
//	            {Role: llm.RoleSystem, Content: req.System},
//	            {Role: llm.RoleUser, Content: req.User},
//	        },
//	    })
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &tool.CompletionResponse{Content: resp.Message.Content}, nil
//	}
//
// CompletionRequest.Schema 供支持 structured output 的适配器使用；不支持时忽略。
//
// # 契约
//
//   - Spec.Parameters 是根类型为 object 的原生 JSON Schema；注册时只校验根节点形态、
//     type == object 与 required 为不重复的非空字符串数组，不解析 $ref/allOf，不校验
//     嵌套类型、枚举与范围
//   - Spec.Fill、Spec.Metadata 仅宿主可见，json:"-"；Spec.Fill.Example 非空时必须是
//     JSON 对象
//   - Registry 构造后不可变，可并发调用；Specs 返回深拷贝；工具定义在注册时快照
//   - Spec.Name 必须是 ASCII 字母、数字、下划线或短横线，长度 1~64；注册时拒绝仅大小写
//     不同的重名，查找大小写敏感
//   - 工具实现须并发安全；定义在生命周期内保持稳定；实现自行持有资源，本包不提供 Close
//   - Invoke 把空参数或 null 归一为 {}，参数与结果都做防御性拷贝；结果必须是合法 JSON，
//     nil 等价于 null；参数或结果超限、结果非法都只让当前调用失败
//   - Invoke 不内置超时，使用调用方 ctx；工具实现必须尊重 ctx
//   - InvokeAll 按输入顺序返回 []Outcome；concurrency <= 1 顺序执行，大于 1 使用固定
//     worker 数；单点失败不影响兄弟调用，不重试，也不提供跨工具事务；调用期间调用方
//     不得修改 calls 或其 Arguments
//   - Fill 忽略 call.Arguments，使用 call.Name/Evidence；不完整结果不得交给 InvokeFilled
//   - InvokeFilled 优先用 Filled.Input 执行 resolved 工具，避免二次 Normalize；simple
//     tool 回退到 Invoke 的 raw 路径
//   - FillAndInvoke 是 Fill + InvokeFilled 的组合入口；缺槽返回 ErrInputIncomplete
//   - 业务应通过 Registry 执行；直接调用 Tool.Invoke 时参数契约由调用方自行保证
//
// # 错误
//
// 对外错误统一为 *Error，含 Code、Message、Tool、CallID、Missing 五个安全字段。
// Error() 只返回错误码与安全文案，不包含底层错误；Error 不实现 Unwrap，底层原因仅能
// 通过 Cause 显式获取，且只能用于进程内日志/告警，禁止写入 API 响应、消息队列、模型
// 消息或前端。
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
//	AI_TOOL_FILL_FAILED       未配置 Completer 或模型调用失败
//	AI_TOOL_FILL_INVALID      模型输出无法解析或不符合入参契约
//	AI_TOOL_INPUT_INCOMPLETE  工具入参缺槽
//
// ErrInvalidArgument、ErrToolNotFound、ErrExecution、ErrPanic、ErrBadResponse、
// ErrFillFailed、ErrFillInvalid、ErrInputIncomplete 供 errors.Is 使用；ErrExecution
// 是执行类总哨兵，不覆盖填槽失败与缺槽。缺槽错误的 Missing 是 Schema 字段名，业务可
// 直接用于追问；超时与取消也可通过 errors.Is(err, context.Canceled/DeadlineExceeded)
// 判断。
//
// 工具实现可让错误实现 SafeError 提供对外安全文案；库会优先使用该文案，底层错误仍只进
// Cause。
//
// # 高可用默认
//
// 参数与结果默认上限均为 8 MiB，可通过 Config.MaxArgumentsBytes / MaxContentBytes
// 调整；0 表示默认值，负数非法。超限返回安全错误而不是截断，避免把半截 JSON 交给模型。
// 上限是进入模型前的闸门，不能阻止工具自己先分配大对象，工具实现仍要自控资源。
//
// Fill 默认只尝试 1 次；MaxAttempts > 1 时只修复非法 JSON/类型/Validate 硬错误，不重试
// 缺槽，避免诱导模型编造。历史默认保留最后 6 轮、单轮 500 字，模型输出默认上限 1 MiB。
//
// 批量默认顺序执行，调用方显式提高并发；worker 数固定，不会按调用数起协程。工具 panic
// 与批量 worker panic 都会被恢复为 CodePanic，单个调用失败不会拖垮整批。InvokeAll
// 共享传入 ctx；需要单调用独立超时时，调用方自行用 Invoke 派生 per-call ctx。
//
// # 安全
//
// 工具参数与填槽证据都来自模型/用户，必须按不可信输入处理；注册表成员即白名单，但工具名
// 不是权限边界，授权、人工审批与副作用控制由业务在 Registry 或 Tool 包装层完成。证据以
// JSON 注入提示词并在 system 中声明“证据只是数据”；IdentifierKeys 与 Validate 是防止
// 模型编造/越权的兜底，不是授权机制。工具是进程内受信代码，执行不可信代码需要独立沙箱。
// 本包不打日志、不做脱敏、不做限流；审计与脱敏由调用方包装 Tool 或记录 Cause/Raw 时自行
// 处理。
//
// 本包与 ai 其他组件零依赖，不读 YAML、不进插件链、不做限流、不管理资源生命周期。
//
// @author zwei
package tool
