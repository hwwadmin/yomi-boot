// Package enums 提供跨模块复用的枚举定义
//
// @author zwei
package enums

// DbType 表示数据库类型
//
// 该类型遵循 const + string 枚举模式（Go 规范 §3.5.4），但有意未实现 IsValid 与
// UnmarshalJSON 方法，原因如下：
//   - DbType 不参与 JSON 请求/响应；YAML 由 plugin/builtin/db 的 BootConfig.type 绑定后写入运行时 db.Config，运行时 Config 不写 mapstructure
//   - 合法性校验由 db.Config.Validate 完成（component/common/db/config.go），
//     校验时机更早且报错信息更贴近配置语义
type DbType string

// 数据库类型枚举
const (
	MySQL    DbType = "mysql"
	Postgres DbType = "postgres"
)
