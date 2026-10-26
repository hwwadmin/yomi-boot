// 定义注册表构造、调用与批量执行。
//
// @author zwei

package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime/debug"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
)

const (
	// DefaultMaxArgumentsBytes 是参数默认上限；0 表示使用该值
	DefaultMaxArgumentsBytes = 8 << 20
	// DefaultMaxContentBytes 是结果内容默认上限；0 表示使用该值
	DefaultMaxContentBytes = 8 << 20
)

// Config 创建 Registry 的构造配置；本包不读 YAML
type Config struct {
	Tools             []Tool     // Tools 是注册的工具集合；名称必须唯一
	Completer         Completer  // Completer 是填槽模型端口；nil 时 Fill 返回未配置
	Fill              FillConfig // Fill 是填槽行为配置
	MaxArgumentsBytes int        // MaxArgumentsBytes 是单次调用参数字节上限；0 用默认值，负数非法
	MaxContentBytes   int        // MaxContentBytes 是单次调用结果字节上限；0 用默认值，负数非法
}

// entry 是注册表内部快照；构造后只读
type entry struct {
	tool         Tool          // tool 是工具实现
	spec         Spec          // spec 是构造时快照的规范定义
	required     []string      // required 是 Schema 根级必填参数键
	fillRequired []string      // fillRequired 是填槽运行时必填键
	fillable     bool          // fillable 表示工具声明了填槽规则或实现 InputResolver
	resolver     InputResolver // resolver 非空时由工具负责 typed 入参解析
	resolved     ResolvedTool  // resolved 非空时支持直接执行 typed 入参
}

// Registry 不可变工具注册表；构造后可并发调用
type Registry struct {
	entries           map[string]*entry // entries 以规范名索引
	order             []*entry          // order 保留注册顺序
	completer         Completer         // completer 是填槽模型端口
	fillConfig        FillConfig        // fillConfig 是填槽行为配置
	maxArgumentsBytes int               // maxArgumentsBytes 是参数字节上限
	maxContentBytes   int               // maxContentBytes 是结果字节上限
}

// New 校验配置与工具定义并构造注册表；任一工具非法时整体失败
func New(cfg Config) (*Registry, error) {
	maxArguments, err := resolveLimit(cfg.MaxArgumentsBytes, DefaultMaxArgumentsBytes)
	if err != nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, "", "", err)
	}
	maxContent, err := resolveLimit(cfg.MaxContentBytes, DefaultMaxContentBytes)
	if err != nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, "", "", err)
	}
	fillConfig, err := resolveFillConfig(cfg.Fill)
	if err != nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, "", "", err)
	}
	registry := &Registry{
		entries:           make(map[string]*entry, len(cfg.Tools)),
		completer:         cfg.Completer,
		fillConfig:        fillConfig,
		maxArgumentsBytes: maxArguments,
		maxContentBytes:   maxContent,
	}
	seen := make(map[string]string, len(cfg.Tools))
	for _, current := range cfg.Tools {
		if current == nil {
			return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, "", "", errors.New("ai/tool: 工具为 nil"))
		}
		spec, err := specOf(current)
		if err != nil {
			return nil, err
		}
		normalized, required, err := normalizeSpec(spec)
		if err != nil {
			return nil, err
		}
		// 模型对工具名大小写不敏感，仅大小写不同的名字视为冲突
		key := strings.ToLower(normalized.Name)
		if previous, ok := seen[key]; ok {
			return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "", errors.Newf("ai/tool: 工具名 %q 与 %q 重复", normalized.Name, previous))
		}
		seen[key] = normalized.Name
		resolver, _ := current.(InputResolver)
		resolved, _ := current.(ResolvedTool)
		if normalized.Fill.EvidenceOnly && resolver == nil {
			return nil, newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, normalized.Name, "",
				errors.New("ai/tool: Fill.EvidenceOnly 需要 InputResolver"))
		}
		fillRequired := normalized.Fill.Required
		if len(fillRequired) == 0 {
			fillRequired = required
		}
		registered := &entry{
			tool:         current,
			spec:         normalized,
			required:     required,
			fillRequired: fillRequired,
			fillable:     !fillSpecEmpty(normalized.Fill) || resolver != nil,
			resolver:     resolver,
			resolved:     resolved,
		}
		registry.entries[normalized.Name] = registered
		registry.order = append(registry.order, registered)
	}
	return registry, nil
}

// MustNew 创建 Registry，失败时记录安全错误与底层原因并立即崩溃
func MustNew(cfg Config) *Registry {
	registry, err := New(cfg)
	if err != nil {
		logger.ErrorWithoutCtx("ai/tool 初始化失败", "error", err, "cause", errors.Stack(causeOf(err)))
		panic(err)
	}
	return registry
}

// Specs 按注册顺序返回工具定义深拷贝
func (r *Registry) Specs() []Spec {
	if r == nil {
		return nil
	}
	specs := make([]Spec, 0, len(r.order))
	for _, current := range r.order {
		specs = append(specs, cloneSpec(current.spec))
	}
	return specs
}

// Lookup 按工具名查找实现；名称 trim 后大小写敏感匹配
func (r *Registry) Lookup(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	current, ok := r.entries[strings.TrimSpace(name)]
	if !ok {
		return nil, false
	}
	return current.tool, true
}

// Invoke 执行一次工具调用；所有错误均为 *Error，底层原因只进 Cause
func (r *Registry) Invoke(ctx context.Context, call Call) (*Result, error) {
	name := strings.TrimSpace(call.Name)
	if r == nil {
		return nil, newError(CodeInvalidArgument, defaultInvalidConfigMessage, name, call.ID, errors.New("ai/tool: 注册表未初始化"))
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, name, call.ID)
	}
	if name == "" {
		return nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, "", call.ID, errors.New("ai/tool: 工具名为空"))
	}
	current, ok := r.entries[name]
	if !ok {
		return nil, newError(CodeToolNotFound, defaultToolNotFoundMessage, name, call.ID, errors.Newf("ai/tool: 工具 %q 未注册", name))
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError(err, current.spec.Name, call.ID)
	}
	arguments, object, err := normalizeArguments(call.Arguments, r.argumentsLimit(), current.spec.Name, call.ID)
	if err != nil {
		return nil, err
	}
	// 实现了 InputResolver 的工具自行校验 typed 入参；simple fillable 走 Fill 规则；
	// 其余工具沿用 Schema required
	if current.resolver == nil {
		if current.fillable {
			missing := rawRequiredMissing(object, current.fillRequired)
			missing = append(missing, rawIdentifierMissing(object, current.spec.Fill.IdentifierKeys, call.Evidence)...)
			missing = normalizeMissing(missing)
			if len(missing) > 0 {
				return nil, newIncompleteError(current.spec.Name, call.ID, missing)
			}
		} else if err := validateRequired(current.required, object, current.spec.Name, call.ID); err != nil {
			return nil, err
		}
	}
	normalizedCall := Call{ID: call.ID, Name: current.spec.Name, Arguments: arguments, Evidence: call.Evidence}
	content, err := invokeTool(ctx, current.tool, normalizedCall, current.spec.Name, call.ID)
	if err != nil {
		return nil, err
	}
	content, err = normalizeContent(content, r.contentLimit(), current.spec.Name, call.ID)
	if err != nil {
		return nil, err
	}
	return &Result{CallID: call.ID, Name: current.spec.Name, Content: content}, nil
}

// InvokeAll 按输入顺序执行一批调用；concurrency <= 1 顺序执行，单点失败不影响其他调用
func (r *Registry) InvokeAll(ctx context.Context, calls []Call, concurrency int) []Outcome {
	if len(calls) == 0 {
		return nil
	}
	if r == nil {
		outcomes := make([]Outcome, len(calls))
		for i, call := range calls {
			outcomes[i] = Outcome{
				Call: call,
				Err:  newError(CodeInvalidArgument, defaultInvalidConfigMessage, strings.TrimSpace(call.Name), call.ID, errors.New("ai/tool: 注册表未初始化")),
			}
		}
		return outcomes
	}
	if err := ctx.Err(); err != nil {
		outcomes := make([]Outcome, len(calls))
		for i, call := range calls {
			outcomes[i] = Outcome{Call: call, Err: contextError(err, strings.TrimSpace(call.Name), call.ID)}
		}
		return outcomes
	}
	workers := concurrency
	if workers <= 1 {
		workers = 1
	}
	if workers > len(calls) {
		workers = len(calls)
	}
	outcomes := make([]Outcome, len(calls))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				outcomes[index] = r.safeInvokeOutcome(ctx, calls[index])
			}
		}()
	}
	for i := range calls {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return outcomes
}

// safeInvokeOutcome 在 worker 边界兜底 panic，避免库内部异常打崩进程
func (r *Registry) safeInvokeOutcome(ctx context.Context, call Call) (outcome Outcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = Outcome{
				Call: call,
				Err: newError(CodePanic, defaultPanicMessage, strings.TrimSpace(call.Name), call.ID,
					errors.Newf("ai/tool: 批量执行 panic: %v\n%s", recovered, debug.Stack())),
			}
		}
	}()
	result, err := r.Invoke(ctx, call)
	return Outcome{Call: call, Result: result, Err: err}
}

// invokeTool 调用工具实现并恢复 panic；handler 错误统一包装为对外错误
func invokeTool(ctx context.Context, current Tool, call Call, toolName, callID string) (content json.RawMessage, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			content = nil
			err = newError(CodePanic, defaultPanicMessage, toolName, callID,
				errors.Newf("ai/tool: 工具执行 panic: %v\n%s", recovered, debug.Stack()))
		}
	}()
	content, err = current.Invoke(ctx, call)
	if err != nil {
		return nil, classifyHandlerError(err, toolName, callID)
	}
	return content, nil
}

// normalizeArguments 校验参数上限与对象形态，返回拷贝与解析后的对象
func normalizeArguments(raw json.RawMessage, maxBytes int, toolName, callID string) (json.RawMessage, map[string]json.RawMessage, error) {
	if maxBytes > 0 && len(raw) > maxBytes {
		return nil, nil, newError(CodeInvalidArgument, defaultArgumentsTooLargeMessage, toolName, callID,
			errors.Newf("ai/tool: 参数 %d 字节超过上限 %d", len(raw), maxBytes))
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte("{}")
	}
	arguments := append(json.RawMessage(nil), trimmed...)
	if len(arguments) == 0 || arguments[0] != '{' {
		return nil, nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, toolName, callID,
			errors.New("ai/tool: 参数必须是 JSON 对象"))
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return nil, nil, newError(CodeInvalidArgument, defaultInvalidArgumentMessage, toolName, callID,
			errors.Wrap(err, "ai/tool: 参数不是合法 JSON 对象"))
	}
	return arguments, object, nil
}

// validateRequired 校验根级 required 键存在；不做类型与嵌套校验
func validateRequired(required []string, object map[string]json.RawMessage, toolName, callID string) error {
	for _, key := range required {
		if _, ok := object[key]; ok {
			continue
		}
		return newError(CodeInvalidArgument, defaultInvalidArgumentMessage, toolName, callID,
			errors.Newf("ai/tool: 缺少必填参数 %q", key))
	}
	return nil
}

// normalizeContent 校验结果上限与 JSON 合法性，返回拷贝；空结果归一为 null
func normalizeContent(raw json.RawMessage, maxBytes int, toolName, callID string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("null"), nil
	}
	if maxBytes > 0 && len(trimmed) > maxBytes {
		return nil, newError(CodeBadResponse, defaultContentTooLargeMessage, toolName, callID,
			errors.Newf("ai/tool: 返回内容 %d 字节超过上限 %d", len(trimmed), maxBytes))
	}
	if !json.Valid(trimmed) {
		return nil, newError(CodeBadResponse, defaultBadResponseMessage, toolName, callID,
			errors.New("ai/tool: 返回内容不是合法 JSON"))
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

// resolveLimit 回退默认大小上限并拒绝负数
func resolveLimit(value, fallback int) (int, error) {
	if value < 0 {
		return 0, errors.Newf("ai/tool: 大小上限不能为负: %d", value)
	}
	if value == 0 {
		return fallback, nil
	}
	return value, nil
}

// resolveFillConfig 回退填槽默认值并拒绝负数
func resolveFillConfig(cfg FillConfig) (FillConfig, error) {
	if cfg.MaxAttempts < 0 {
		return FillConfig{}, errors.Newf("ai/tool: Fill.MaxAttempts 不能为负: %d", cfg.MaxAttempts)
	}
	if cfg.MaxHistoryTurns < 0 {
		return FillConfig{}, errors.Newf("ai/tool: Fill.MaxHistoryTurns 不能为负: %d", cfg.MaxHistoryTurns)
	}
	if cfg.MaxHistoryRunes < 0 {
		return FillConfig{}, errors.Newf("ai/tool: Fill.MaxHistoryRunes 不能为负: %d", cfg.MaxHistoryRunes)
	}
	if cfg.MaxResponseBytes < 0 {
		return FillConfig{}, errors.Newf("ai/tool: Fill.MaxResponseBytes 不能为负: %d", cfg.MaxResponseBytes)
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = DefaultMaxFillAttempts
	}
	if cfg.MaxHistoryTurns == 0 {
		cfg.MaxHistoryTurns = DefaultMaxFillHistoryTurns
	}
	if cfg.MaxHistoryRunes == 0 {
		cfg.MaxHistoryRunes = DefaultMaxFillHistoryRunes
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = DefaultMaxFillResponseBytes
	}
	return cfg, nil
}

// argumentsLimit 返回生效的参数字节上限
func (r *Registry) argumentsLimit() int {
	if r == nil || r.maxArgumentsBytes <= 0 {
		return DefaultMaxArgumentsBytes
	}
	return r.maxArgumentsBytes
}

// contentLimit 返回生效的结果字节上限
func (r *Registry) contentLimit() int {
	if r == nil || r.maxContentBytes <= 0 {
		return DefaultMaxContentBytes
	}
	return r.maxContentBytes
}

// specOf 在构造期读取工具定义并兜底 panic，坏工具只让 New 返回错误
func specOf(current Tool) (spec Spec, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = newError(CodeInvalidArgument, defaultInvalidDefinitionMessage, "", "",
				errors.Newf("ai/tool: Spec 执行 panic: %v\n%s", recovered, debug.Stack()))
		}
	}()
	return current.Spec(), nil
}

// causeOf 取出对外错误的底层原因；非 *Error 时原样返回
func causeOf(err error) error {
	var toolErr *Error
	if errors.As(err, &toolErr) && toolErr != nil {
		return toolErr.Cause()
	}
	return err
}
