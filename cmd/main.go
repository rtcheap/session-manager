package main

import (
	"net/http"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/logger"
	_ "github.com/go-sql-driver/mysql"
)

var log = logger.GetDefaultLogger("session-manager/main")

func main() {
	log.Info("Hello World")
}

func newServer(e *env) *http.Server {
	r := httputil.NewRouter("session-manager", e.checkHealth)

	r.POST("/v1/sessions", notImplemented)           // start session
	r.PUT("/v1/sessions/:sessionId", notImplemented) // join session

	return &http.Server{
		Addr:    ":" + e.cfg.port,
		Handler: r,
	}
}
