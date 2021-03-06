package main

import (
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/environ"
	"github.com/CzarSimon/httputil/jwt"
)

type config struct {
	db             dbutil.Config
	port           string
	migrationsPath string
	jwtCredentials jwt.Credentials
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
		port:           environ.Get("SERVICE_PORT", "8080"),
		migrationsPath: environ.Get("MIGRATIONS_PATH", "/etc/service-registry/migrations"),
		jwtCredentials: getJwtCredentials(),
	}
}

func getJwtCredentials() jwt.Credentials {
	return jwt.Credentials{
		Issuer: environ.MustGet("JWT_ISSUER"),
		Secret: environ.MustGet("JWT_SECRET"),
	}
}
