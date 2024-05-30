package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config of the bunny-upload
type Config struct {
	Path    string  `yaml:"path"`
	Cache   Cache   `yaml:"cache"`
	Storage Storage `yaml:"storage"`
}

// Cache config
type Cache struct {
	PullZone  int    `yaml:"pull_zone"`
	AccessKey string `yaml:"access_key"`
}

// Storage config
type Storage struct {
	Zone     string `yaml:"storage_zone"`
	Password string `yaml:"password"`
}

// Read config from file system
func Read(configPath string) (*Config, error) {
	configb, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var config Config
	err = yaml.Unmarshal(configb, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
