// Package config 提供基于注册表的配置加载
//
// @author zwei
package config

import (
	"bytes"
	"os"
	"sort"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// LoadConfig 按注册表加载配置：严格根 key → Bind → Validate
func LoadConfig(configPath, envPrefix string, reg *Registry) (*Config, error) {
	if reg == nil {
		return nil, errors.New("配置注册表不能为空")
	}
	if len(reg.sections) == 0 {
		return nil, errors.New("配置注册表不能为空段")
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yml")
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrap(err, "加载配置文件失败")
	}
	expanded := ExpandEnvDefaults(string(raw))

	if err := v.ReadConfig(bytes.NewReader([]byte(expanded))); err != nil {
		return nil, errors.Wrap(err, "加载配置文件失败")
	}

	rootKeys, err := yamlRootKeys(expanded)
	if err != nil {
		return nil, errors.Wrap(err, "解析配置根 key 失败")
	}
	if unknown := unknownRootKeys(rootKeys, reg); len(unknown) > 0 {
		return nil, errors.New(formatUnknownRootKeys(unknown))
	}

	for _, s := range reg.sections {
		if err := v.UnmarshalKey(s.Key(), s.Target()); err != nil {
			return nil, errors.Wrap(err, "解析配置段失败: %s", s.Key())
		}
	}

	cfg := &Config{
		sections: make(map[string]Section, len(reg.sections)),
		order:    make([]string, 0, len(reg.sections)),
	}
	for _, s := range reg.sections {
		cfg.sections[s.Key()] = s
		cfg.order = append(cfg.order, s.Key())
	}

	for _, s := range reg.sections {
		if err := s.Validate(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// yamlRootKeys 从已展开 YAML 文本提取根层 key
func yamlRootKeys(expanded string) ([]string, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(expanded), &root); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	return keys, nil
}

// unknownRootKeys 返回 YAML 中未注册的根 key（排序后便于稳定报错）
func unknownRootKeys(rootKeys []string, reg *Registry) []string {
	var unknown []string
	for _, key := range rootKeys {
		if !reg.Has(key) {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}
