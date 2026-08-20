package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Server   ServerSection   `mapstructure:"server"`
	DB       DBSection       `mapstructure:"db"`
	Storage  StorageSection  `mapstructure:"storage"`
	Auth     AuthSection     `mapstructure:"auth"`
	Audit    AuditSection    `mapstructure:"audit"`
	Log      LogSection      `mapstructure:"log"`
	CORS     CORSSection     `mapstructure:"cors"`
	WSS      WSSSection      `mapstructure:"wss"`
	Upload   UploadSection   `mapstructure:"upload"`
	Cloud    CloudSection    `mapstructure:"cloud"`
}

type ServerSection struct {
	Listen     string     `mapstructure:"listen"`
	Domain     string     `mapstructure:"domain"`
	TLS        TLSSection `mapstructure:"tls"`
}

type TLSSection struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type DBSection struct {
	Path string `mapstructure:"path"`
}

type StorageSection struct {
	DataDir           string `mapstructure:"data_dir"`
	DocRetentionHours int    `mapstructure:"doc_retention_hours"`
}

type AuthSection struct {
	JWTTTLHours  int    `mapstructure:"jwt_ttl_hours"`
	BcryptCost   int    `mapstructure:"bcrypt_cost"`
	JWTSecret    string `mapstructure:"-"`
	MasterKey    string `mapstructure:"-"`
}

type AuditSection struct {
	RetentionDays int `mapstructure:"retention_days"`
}

type LogSection struct {
	Level         string `mapstructure:"level"`
	RetentionDays int    `mapstructure:"retention_days"`
	MaxSizeMB     int    `mapstructure:"max_size_mb"`
	Dir           string `mapstructure:"dir"`
}

type CORSSection struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

type WSSSection struct {
	Path             string        `mapstructure:"path"`
	HeartbeatInterval time.Duration `mapstructure:"-"`
	HeartbeatTimeout  time.Duration `mapstructure:"-"`
	MinVersion       int           `mapstructure:"min_version"`
}

type UploadSection struct {
	MaxSizeMB int `mapstructure:"max_size_mb"`
}

type CloudSection struct {
	Endpoint        string `mapstructure:"endpoint"`
	AgentTokenDefault string `mapstructure:"agent_token_default"`
}

func Load(path string) (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("CPS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &ServerConfig{}
	decoderConfig := mapstructure.DecoderConfig{
		TagName:  "mapstructure",
		Result:   cfg,
		WeaklyTypedInput: true,
	}
	decoder, err := mapstructure.NewDecoder(&decoderConfig)
	if err != nil {
		return nil, fmt.Errorf("new decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	applyFixedDefaults(cfg)

	cfg.Auth.JWTSecret = os.Getenv("CPS_JWT_SECRET")
	cfg.Auth.MasterKey = os.Getenv("CPS_MASTER_KEY")

	return cfg, nil
}

func applyFixedDefaults(cfg *ServerConfig) {
	cfg.WSS.Path = "/agent"
	cfg.WSS.HeartbeatInterval = 30 * time.Second
	cfg.WSS.HeartbeatTimeout = 90 * time.Second
	if cfg.WSS.MinVersion == 0 {
		cfg.WSS.MinVersion = 1
	}
	cfg.Upload.MaxSizeMB = 50
	if cfg.Auth.BcryptCost == 0 {
		cfg.Auth.BcryptCost = 10
	}
	if cfg.Auth.JWTTTLHours == 0 {
		cfg.Auth.JWTTTLHours = 12
	}
	if cfg.Audit.RetentionDays == 0 {
		cfg.Audit.RetentionDays = 180
	}
	if cfg.Log.RetentionDays == 0 {
		cfg.Log.RetentionDays = 30
	}
	if cfg.Log.MaxSizeMB == 0 {
		cfg.Log.MaxSizeMB = 100
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Storage.DocRetentionHours == 0 {
		cfg.Storage.DocRetentionHours = 24
	}
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if len(cfg.CORS.AllowedOrigins) == 0 {
		cfg.CORS.AllowedOrigins = []string{"https://print.oascii.com"}
	}
}

func Validate(cfg *ServerConfig) error {
	if cfg.Server.Listen == "" {
		return fmt.Errorf("server.listen 必填")
	}
	if cfg.Server.Domain == "" {
		return fmt.Errorf("server.domain 必填")
	}
	if !isValidDomain(cfg.Server.Domain) {
		return fmt.Errorf("云端地址必须为域名，禁止 IP: %s", cfg.Server.Domain)
	}
	if cfg.DB.Path == "" {
		return fmt.Errorf("db.path 必填")
	}
	parent := filepath.Dir(cfg.DB.Path)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return fmt.Errorf("db.path 父目录不存在或不可写: %s", parent)
	}
	if cfg.Auth.JWTTTLHours < 1 || cfg.Auth.JWTTTLHours > 720 {
		return fmt.Errorf("auth.jwt_ttl_hours 必须在 1-720 之间")
	}
	if cfg.Audit.RetentionDays < 30 || cfg.Audit.RetentionDays > 365 {
		return fmt.Errorf("audit.retention_days 必须在 30-365 之间")
	}
	if cfg.Log.RetentionDays < 7 || cfg.Log.RetentionDays > 90 {
		return fmt.Errorf("log.retention_days 必须在 7-90 之间")
	}
	if cfg.Auth.JWTSecret == "" {
		return fmt.Errorf("密钥未配置: CPS_JWT_SECRET 环境变量为空")
	}
	if cfg.Auth.MasterKey == "" {
		return fmt.Errorf("密钥未配置: CPS_MASTER_KEY 环境变量为空")
	}
	return nil
}

func isValidDomain(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, " /") {
		return false
	}
	if !strings.Contains(s, ".") {
		return false
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	allDigit := true
	for _, p := range parts {
		for _, c := range p {
			if c < '0' || c > '9' {
				allDigit = false
				break
			}
		}
		if !allDigit {
			break
		}
	}
	if allDigit {
		return false
	}
	return true
}