package main

import (
	"database/sql"
	"io"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"go.uber.org/zap"
)

type env struct {
	cfg         config
	db          *sql.DB
	traceCloser io.Closer
}

func (e *env) checkHealth() error {
	err := dbutil.Connected(e.db)
	if err != nil {
		return httputil.ServiceUnavailableError(err)
	}

	return nil
}

func (e *env) close() {
	err := e.db.Close()
	if err != nil {
		log.Error("failed to close database connection", zap.Error(err))
	}

	err = e.traceCloser.Close()
	if err != nil {
		log.Error("failed to close tracer connection", zap.Error(err))
	}
}

func setupEnv() *env {
	jcfg, err := jaegercfg.FromEnv()
	if err != nil {
		log.Fatal("failed to create jaeger configuration", zap.Error(err))
	}

	tracer, closer, err := jcfg.NewTracer()
	if err != nil {
		log.Fatal("failed to create tracer", zap.Error(err))
	}

	opentracing.SetGlobalTracer(tracer)

	cfg := getConfig()
	db := dbutil.MustConnect(cfg.db)
	err = dbutil.Upgrade(cfg.migrationsPath, cfg.db.Driver(), db)
	if err != nil {
		log.Fatal("failed to apply database migrations", zap.Error(err))
	}

	return &env{
		cfg:         cfg,
		db:          db,
		traceCloser: closer,
	}
}

func notImplemented(c *gin.Context) {
	err := httputil.NotImplementedError(nil)
	c.Error(err)
}
