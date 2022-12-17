package config

import "os"

func loadEnvWithDefault(envKey, defaultValue string) string {
	value, ok := os.LookupEnv(envKey)

	if ok {
		return value
	}

	return defaultValue
}
