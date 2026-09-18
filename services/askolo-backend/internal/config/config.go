package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = 8090
)

type Config struct {
	ServiceName         string
	Environment         string
	Host                string
	Port                int
	InternalAuthToken   string
}

func Load() (Config, error) {
	port, err := envPort("PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}

	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("NODE_ENV"))
	}
	if environment == "" {
		environment = "development"
	}

	host := strings.TrimSpace(os.Getenv("BACKEND_HOST"))
	if host == "" {
		host = defaultHost
	}

	return Config{
		ServiceName:       "askolo-backend",
		Environment:       environment,
		Host:              host,
		Port:              port,
		InternalAuthToken: strings.TrimSpace(os.Getenv("ASKOLO_INTERNAL_TOKEN")),
	}, nil
}

func envPort(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be a valid TCP port", name)
	}
	return port, nil
}