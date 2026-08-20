package config

import "fmt"

func (c *ServerConfig) Validate() error {
	return Validate(c)
}

func (c *ServerConfig) ValidateDomain() error {
	if c.Server.Domain == "" {
		return fmt.Errorf("server.domain 必填")
	}
	if !isValidDomain(c.Server.Domain) {
		return fmt.Errorf("云端地址必须为域名，禁止 IP: %s", c.Server.Domain)
	}
	return nil
}

func (c *ServerConfig) ValidateSecrets() error {
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("密钥未配置: CPS_JWT_SECRET 环境变量为空")
	}
	if c.Auth.MasterKey == "" {
		return fmt.Errorf("密钥未配置: CPS_MASTER_KEY 环境变量为空")
	}
	return nil
}