package config

import (
	"os"
	"strconv"
)

type EnvironmentValue interface {
	string | int | bool
}

func GetEnv(key string) string {
	return os.Getenv(key)
}

func GetEnvOrDefault(key string, defaultValue string) string {
	value := GetEnv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func GetIntegerOrDefault(key string, defaultValue int) int {
	strValue := GetEnv(key)
	value, err := strconv.Atoi(strValue)
	if err != nil {
		return defaultValue
	}
	return value
}
