package main

import (
	"strconv"

	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/environ"
	"github.com/CzarSimon/httputil/jwt"
	"go.uber.org/zap"
)

type config struct {
	db              dbutil.Config
	port            string
	sessionRegistry sessionRegistryConfig
	turn            turnConfig
	migrationsPath  string
	jwtCredentials  jwt.Credentials
}

type sessionRegistryConfig struct {
	url string
}

type turnConfig struct {
	udpPort     int
	rpcProtocol string
}

func getConfig() config {
	return config{
		db: dbutil.MysqlConfig{
			Host:             environ.MustGet("DB_HOST"),
			Port:             environ.MustGet("DB_PORT"),
			Database:         environ.MustGet("DB_DATABASE"),
			User:             environ.MustGet("DB_USERNAME"),
			Password:         environ.MustGet("DB_PASSWORD"),
			ConnectionParams: "parseTime=true",
		},
		port:            environ.Get("SERVICE_PORT", "8080"),
		turn:            getTurnConfig(),
		sessionRegistry: getSessionRegistryConfig(),
		migrationsPath:  environ.Get("MIGRATIONS_PATH", "/etc/service-registry/migrations"),
		jwtCredentials:  getJwtCredentials(),
	}
}

func getTurnConfig() turnConfig {
	udpPort, err := strconv.Atoi(environ.Get("TURN_UDP_PORT", "3478"))
	if err != nil {
		log.Fatal("failed to parse turn udp port", zap.Error(err))
	}

	return turnConfig{
		udpPort:     udpPort,
		rpcProtocol: environ.Get("TURN_RPC_PROTOCOL", "http"),
	}
}

func getSessionRegistryConfig() sessionRegistryConfig {
	return sessionRegistryConfig{
		url: environ.Get("SESSIONREGISTRY_URL", "http://session-registry:8080"),
	}
}

func getJwtCredentials() jwt.Credentials {
	return jwt.Credentials{
		Issuer: environ.MustGet("JWT_ISSUER"),
		Secret: environ.MustGet("JWT_SECRET"),
	}
}
