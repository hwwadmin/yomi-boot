// 验证码配置、驱动标识与默认值；本包不读 YAML。
//
// @author zwei

package captcha

import (
	"strings"
	"time"
)

// Driver 验证码驱动标识
type Driver string

const (
	// DriverDigit 数字验证码；默认
	DriverDigit Driver = "digit"
	// DriverString 字符验证码
	DriverString Driver = "string"
	// DriverMath 算术验证码；答案为算式结果，Length 忽略
	DriverMath Driver = "math"
)

// LineOption 干扰线组合（按位或）
type LineOption uint8

const (
	// LineHollow 空心线
	LineHollow LineOption = 1 << iota
	// LineSlime 细直线
	LineSlime
	// LineSine 正弦线
	LineSine
)

const (
	defaultLength = 4
	defaultWidth  = 240
	defaultHeight = 80
	defaultTTL    = 5 * time.Minute

	minLength = 3
	maxLength = 8
	minWidth  = 120
	maxWidth  = 400
	minHeight = 60
	maxHeight = 160

	maxNoiseCount = 100

	minTTL = 30 * time.Second
	maxTTL = 10 * time.Minute

	defaultSource = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	minSourceLen  = 8
	maxSourceLen  = 128
)

// Config 表示验证码运行时配置；零值字段按默认值处理
type Config struct {
	Driver     Driver        // Driver 是驱动标识；空则 digit
	Length     int           // Length 是答案字符数；digit/string 有效，0 则 4（3..8）；math 忽略
	Width      int           // Width 是图片宽度（像素）；0 则 240（120..400）
	Height     int           // Height 是图片高度（像素）；0 则 80（60..160）
	NoiseCount int           // NoiseCount 是背景干扰字符数；string/math 有效，默认 0（0..100）
	Lines      LineOption    // Lines 是干扰线组合；string/math 有效，默认无线
	Source     string        // Source 是 string 驱动的字符源；空则内置字符集；仅 ASCII 字母数字
	TTL        time.Duration // TTL 是验证码有效期；0 则 5m（30s..10m）
}

// Validate 校验配置语义；零值字段按默认值处理
func (c Config) Validate() error {
	_, err := c.normalize()
	return err
}

// normalize 返回填充默认值后的配置副本
func (c Config) normalize() (Config, error) {
	normalized := c
	if normalized.Driver == "" {
		normalized.Driver = DriverDigit
	}
	switch normalized.Driver {
	case DriverDigit, DriverString, DriverMath:
	default:
		return Config{}, invalidArgument("captcha: driver 仅支持 digit / string / math，实际为 %q", c.Driver)
	}

	if normalized.TTL == 0 {
		normalized.TTL = defaultTTL
	}
	if normalized.TTL < minTTL || normalized.TTL > maxTTL {
		return Config{}, invalidArgument("captcha: ttl 必须在 %s..%s 之间", minTTL, maxTTL)
	}

	if normalized.Width == 0 {
		normalized.Width = defaultWidth
	}
	if normalized.Width < minWidth || normalized.Width > maxWidth {
		return Config{}, invalidArgument("captcha: width 必须在 %d..%d 之间", minWidth, maxWidth)
	}

	if normalized.Height == 0 {
		normalized.Height = defaultHeight
	}
	if normalized.Height < minHeight || normalized.Height > maxHeight {
		return Config{}, invalidArgument("captcha: height 必须在 %d..%d 之间", minHeight, maxHeight)
	}

	switch normalized.Driver {
	case DriverDigit, DriverString:
		if normalized.Length == 0 {
			normalized.Length = defaultLength
		}
		if normalized.Length < minLength || normalized.Length > maxLength {
			return Config{}, invalidArgument("captcha: length 必须在 %d..%d 之间", minLength, maxLength)
		}
	}

	if normalized.Driver == DriverString {
		if strings.TrimSpace(normalized.Source) == "" {
			normalized.Source = defaultSource
		}
		if err := validateSource(normalized.Source); err != nil {
			return Config{}, err
		}
	}

	if normalized.Driver == DriverString || normalized.Driver == DriverMath {
		if normalized.NoiseCount < 0 || normalized.NoiseCount > maxNoiseCount {
			return Config{}, invalidArgument("captcha: noise_count 必须在 0..%d 之间", maxNoiseCount)
		}
		if normalized.Lines&^(LineHollow|LineSlime|LineSine) != 0 {
			return Config{}, invalidArgument("captcha: lines 含未知的干扰线类型")
		}
	}

	return normalized, nil
}

// validateSource 校验字符源仅含 ASCII 字母数字且长度合理
func validateSource(source string) error {
	if len(source) < minSourceLen || len(source) > maxSourceLen {
		return invalidArgument("captcha: source 长度必须在 %d..%d 之间", minSourceLen, maxSourceLen)
	}
	for _, r := range source {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		return invalidArgument("captcha: source 仅允许 ASCII 字母和数字")
	}
	return nil
}
