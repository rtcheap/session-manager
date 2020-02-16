package main

import (
	"net/http"
	"time"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/jwt"
	"github.com/CzarSimon/httputil/logger"
	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

var log = logger.GetDefaultLogger("session-manager/main")

func main() {
	e := setupEnv()
	defer e.close()

	server := newServer(e)
	log.Info("Started session-manager listening on port: " + e.cfg.port)

	err := server.ListenAndServe()
	if err != nil {
		log.Error("Unexpected error stoped server.", zap.Error(err))
	}
}

func newServer(e *env) *http.Server {
	r := httputil.NewRouter("session-manager", e.checkHealth)

	r.POST("/v1/sessions", e.createSession)
	r.GET("/v1/sessions/:sessionId", e.joinSession)

	rbac := httputil.RBAC{
		Verifier: jwt.NewVerifier(e.cfg.jwtCredentials, time.Minute),
	}
	messageGroup := r.Group("/v1/sessions/:sessionId/messages", rbac.Secure("USER"))
	messageGroup.POST("/signal", notImplemented)

	return &http.Server{
		Addr:    ":" + e.cfg.port,
		Handler: r,
	}
}
