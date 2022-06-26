package config

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port   int
	Prefix string `yaml:"prefix"`
	DSN    string `yaml:"dsn"`
}

var (
	configPath string

	defaultConfig = Config{
		Port:   50051,
		Prefix: "10.255.255.0/24",
		DSN:    "file:toxfu.db?mode=rwc&cache=shared&_fk=1",
	}
)

func init() {
	flag.StringVar(&configPath, "config", "toxfu.yaml", "Path to toxfu.yaml")
}

func NewConfig() (*Config, error) {
	flag.Parse()

	fp, err := os.Open(configPath)

	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer fp.Close()

	cfg := defaultConfig
	if err := yaml.NewDecoder(fp).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return &cfg, nil
}
