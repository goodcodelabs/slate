package configuration

import (
	"slate/internal/config"
)

type Configuration struct {
	Port              int
	Host              string
	MaxConnections    int
	Timeout           int
	ClientIdleTimeout int
}

func New() *Configuration {
	return &Configuration{
		Port:              config.GetIntegerOrDefault("PORT", 4242),
		Host:              config.GetEnvOrDefault("HOST", "localhost"),
		MaxConnections:    config.GetIntegerOrDefault("MAX_CONNECTIONS", 10),
		Timeout:           config.GetIntegerOrDefault("TIMEOUT", 500),
		ClientIdleTimeout: config.GetIntegerOrDefault("CLIENT_IDLE_TIMEOUT_MS", 60000),
	}
}
