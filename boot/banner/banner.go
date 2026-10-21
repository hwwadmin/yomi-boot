// Package banner 提供服务启动 Banner 输出能力
//
// @author zwei
package banner

import (
	_ "embed"
	"fmt"
)

//go:embed banner.txt
var banner string

// Banner 输出服务启动 Banner 到控制台
func Banner() {
	fmt.Print(banner)
}
