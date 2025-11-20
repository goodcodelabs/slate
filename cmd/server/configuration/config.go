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

func NewConfiguration() *Configuration {
	c := &Configuration{}

	c.Port = config.GetIntegerOrDefault("PORT", 4242)
	c.Host = config.GetEnvOrDefault("HOST", "localhost")
	c.MaxConnections = config.GetIntegerOrDefault("MAX_CONNECTIONS", 10)
	c.Timeout = config.GetIntegerOrDefault("TIMEOUT", 500)
	c.ClientIdleTimeout = config.GetIntegerOrDefault("CLIENT_IDLE_TIMEOUT_MS", 60000)

	return c
}
