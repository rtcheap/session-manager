package main

import (
	"database/sql"
	"io"
	"time"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/client"
	"github.com/CzarSimon/httputil/client/rpc"
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/jwt"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/rtcheap/service-clients/go/serviceregistry"
	"github.com/rtcheap/service-clients/go/turnserver"
	"github.com/rtcheap/session-manager/internal/repository"
	"github.com/rtcheap/session-manager/internal/service"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"go.uber.org/zap"
)

type env struct {
	cfg            config
	db             *sql.DB
	sessionService service.SessionService
	traceCloser    io.Closer
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

	registryClient := serviceregistry.NewClient(client.Client{
		RPCClient: rpc.NewClient(2 * time.Second),
		Issuer:    jwt.NewIssuer(cfg.jwtCredentials),
		BaseURL:   cfg.sessionRegistry.url,
		Role:      jwt.SystemRole,
		UserAgent: "session-manager/serviceregistry.Client",
	})

	turnClient := turnserver.NewClient(client.Client{
		RPCClient: rpc.NewClient(time.Second),
		Issuer:    jwt.NewIssuer(cfg.jwtCredentials),
		Role:      jwt.SystemRole,
		UserAgent: "session-manager/turnserver.Client",
	})

	return &env{
		cfg:         cfg,
		db:          db,
		traceCloser: closer,
		sessionService: service.SessionService{
			RelayPort:       cfg.turn.udpPort,
			TurnRPCProtocol: cfg.turn.rpcProtocol,
			SessionRepo:     repository.NewSessionRepository(db),
			RegistryClient:  registryClient,
			TurnClient:      turnClient,
		},
	}
}

func notImplemented(c *gin.Context) {
	err := httputil.NotImplementedError(nil)
	c.Error(err)
}
