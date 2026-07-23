// Package consul 提供 Consul 服务注册与注销实现
package consul

import (
	"fmt"
	"strings"
	"yt-boot/errors"
	"yt-boot/logger"

	"github.com/hashicorp/consul/api"
)

// RegisterService 将当前实例注册到 Consul Agent
func (c *Client) RegisterService(cfg Config, appName string) (string, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		return "", errors.New("consul 配置不能为空")
	}
	serviceName := cfg.ServiceNameOrDefault(appName)
	serviceHost, servicePort, err := cfg.ServiceHostPort()
	if err != nil {
		return "", errors.Wrap(err, "consul.service_address 无效")
	}
	checkURL, err := cfg.CheckURL()
	if err != nil {
		return "", errors.Wrap(err, "consul.check_address 无效")
	}
	serviceID := cfg.ServiceIDClean()

	reg := &api.AgentServiceRegistration{
		ID:      serviceID,
		Name:    serviceName,
		Port:    servicePort,
		Address: serviceHost,
		Check: &api.AgentServiceCheck{
			HTTP:                           checkURL,
			Interval:                       "10s",
			Timeout:                        "3s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}
	if err := c.api.Agent().ServiceRegister(reg); err != nil {
		return "", errors.Wrap(err, "consul 注册失败")
	}
	logger.InfoWithoutCtx(
		"consul 服务注册成功",
		"consul_addr", cfg.Addr,
		"service_id", serviceID,
		"service_name", serviceName,
		"addr", fmt.Sprintf("%s:%d", serviceHost, servicePort),
		"check_url", checkURL,
	)
	return serviceID, nil
}

// DeregisterService 从 Consul 注销服务实例
func (c *Client) DeregisterService(serviceID string) error {
	id := strings.TrimSpace(serviceID)
	if id == "" {
		return nil
	}
	if err := c.api.Agent().ServiceDeregister(id); err != nil {
		return errors.Wrap(err, "consul 注销失败")
	}
	return nil
}
