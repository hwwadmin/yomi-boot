package sm4

import "gitee.com/hq_project/yomi-boot/errors"

// ErrOpen 表示解密失败（认证失败、密文过短或 padding 非法）
var ErrOpen = errors.NewNoStack("sm4: open failed")
