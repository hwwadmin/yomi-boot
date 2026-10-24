package appctx

// AppCtx 表示应用静态上下文能力接口
type AppCtx interface {
	// Name 返回服务名称
	Name() string
	// Env 返回运行环境
	Env() string
	// Version 返回当前版本号
	Version() string
}

// App 表示服务的静态上下文信息
type App struct {
	name    string
	env     string
	version string
}

var _ AppCtx = (*App)(nil)

// NewAppCtx 创建应用静态上下文对象
func NewAppCtx(name string, env string, version string) *App {
	return &App{
		name:    name,
		env:     env,
		version: version,
	}
}

// Name 返回服务名称
func (a *App) Name() string {
	if a == nil {
		return ""
	}

	return a.name
}

// Env 返回运行环境
func (a *App) Env() string {
	if a == nil {
		return ""
	}

	return a.env
}

// Version 返回当前版本号
func (a *App) Version() string {
	if a == nil {
		return ""
	}

	return a.version
}
