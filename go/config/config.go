package config

import (
	"strings"

	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type StripeConfig struct {
	SecretKey     string `mapstructure:"secret_key"`
	WebhookSecret string `mapstructure:"webhook_secret"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Addr string `mapstructure:"addr"`
}

type AgentConfig struct {
	BillingURL  string `mapstructure:"billing_url"`
	RecoveryURL string `mapstructure:"recovery_url"`
	SupportURL  string `mapstructure:"support_url"`
	AuditURL    string `mapstructure:"audit_url"`
}

type AppConfig struct {
	Server       ServerConfig
	Stripe       StripeConfig
	Postgres     PostgresConfig
	Redis        RedisConfig
	Agents       AgentConfig
	OntologyPath string `mapstructure:"ontology_path"`
}

func LoadConfig() (*AppConfig, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
