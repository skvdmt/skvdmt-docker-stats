package model

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const (
	APP_NAME = "skvdmt-docker-stats"

	MODE_ENV  = "MODE"
	MODE_DEV  = "dev"
	MODE_PROD = "prod"

	CONFIG_DIR_DEV  = "./config"
	CONFIG_DIR_PROD = "/etc"

	CONFIG_FILENAME_DEV  = "dev.yaml"
	CONFIG_FILENAME_PROD = "prod.yaml"
)

// Conf Глобальный экземпляр конфигурации.
var Conf *Config

// ServerConfig Конфигурация сервера.
type ServerConfig struct {
	Port uint16 `yaml:"port"`
}

// Config Конфигурация.
type Config struct {
	Server *ServerConfig `yaml:"server"`
}

// NewConfig Конструктор.
func NewConfig() (*Config, error) {
	Log.Info.Info("configuration loading")
	r, err := os.ReadFile(filepath.Join(configPath()))
	if err != nil {
		return nil, err
	}
	c := &Config{
		Server: &ServerConfig{},
	}
	if err := yaml.Unmarshal(r, c); err != nil {
		return nil, err
	}
	return c, nil
}

// configPath Путь к файлу конфигурации.
func configPath() string {
	if os.Getenv(MODE_ENV) == MODE_DEV {
		return filepath.Join(CONFIG_DIR_DEV, CONFIG_FILENAME_DEV)
	}
	return filepath.Join(CONFIG_DIR_PROD, APP_NAME, CONFIG_FILENAME_PROD)
}
