package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App       AppConfig       `yaml:"app"`
	Log       LogConfig       `yaml:"log"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	JWT       JWTConfig       `yaml:"jwt"`
	Storage   StorageConfig   `yaml:"storage"`
	Transcode TranscodeConfig `yaml:"transcode"`
}

type AppConfig struct {
	Mode string `yaml:"mode"`
	Addr string `yaml:"addr"`
}

type LogConfig struct {
	Level    string `yaml:"level"`
	Dir      string `yaml:"dir"`
	Filename string `yaml:"filename"`
}

type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

type JWTConfig struct {
	Secret       string        `yaml:"secret"`
	Expire       time.Duration `yaml:"-"`
	ExpireString string        `yaml:"expire"`
}

type StorageConfig struct {
	Root string `yaml:"root"`
}

type TranscodeConfig struct {
	Concurrency        int           `yaml:"concurrency"`
	FFmpeg             string        `yaml:"ffmpeg"`
	FFprobe            string        `yaml:"ffprobe"`
	PollInterval       time.Duration `yaml:"-"`
	PollIntervalString string        `yaml:"poll_interval"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(cfg)
	applyEnv(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.App.Mode == "" {
		cfg.App.Mode = "dev"
	}
	if cfg.App.Addr == "" {
		cfg.App.Addr = ":8080"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Dir == "" {
		cfg.Log.Dir = "logs"
	}
	if cfg.Log.Filename == "" {
		cfg.Log.Filename = "app"
	}
	if cfg.JWT.ExpireString != "" {
		if d, err := time.ParseDuration(cfg.JWT.ExpireString); err == nil {
			cfg.JWT.Expire = d
		}
	}
	if cfg.JWT.Expire == 0 {
		cfg.JWT.Expire = 24 * time.Hour
	}
	if cfg.Storage.Root == "" {
		cfg.Storage.Root = "storage"
	}
	if cfg.Transcode.Concurrency <= 0 {
		cfg.Transcode.Concurrency = 2
	}
	if cfg.Transcode.FFmpeg == "" {
		cfg.Transcode.FFmpeg = "ffmpeg"
	}
	if cfg.Transcode.FFprobe == "" {
		cfg.Transcode.FFprobe = "ffprobe"
	}
	if cfg.Transcode.PollIntervalString != "" {
		if d, err := time.ParseDuration(cfg.Transcode.PollIntervalString); err == nil {
			cfg.Transcode.PollInterval = d
		}
	}
	if cfg.Transcode.PollInterval == 0 {
		cfg.Transcode.PollInterval = 5 * time.Second
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("APP_MODE"); v != "" {
		cfg.App.Mode = v
	}
	if v := os.Getenv("APP_ADDR"); v != "" {
		cfg.App.Addr = v
	}
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWT.Secret = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

func (c *Config) IsDev() bool {
	return c.App.Mode == "dev"
}
