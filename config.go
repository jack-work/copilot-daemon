package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Port the copilot-api listens on.
	Port int `json:"port"`

	// If true, do not kill an existing process on the port — just exit.
	DoNotKillExisting bool `json:"do_not_kill_existing"`
}

func defaultConfig() Config {
	return Config{
		Port:              4141,
		DoNotKillExisting: false,
	}
}

func configPath() string {
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func loadConfig() Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	if cfg.Port == 0 {
		cfg.Port = 4141
	}
	return cfg
}
