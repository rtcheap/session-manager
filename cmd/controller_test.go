package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CzarSimon/httputil/client/rpc"
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/id"
	"github.com/CzarSimon/httputil/jwt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/opentracing/opentracing-go"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/session-manager/internal/models"
	"github.com/rtcheap/session-manager/internal/repository"
	"github.com/rtcheap/session-manager/internal/service"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCreateSession_NoCredentials(t *testing.T) {
	assert := assert.New(t)
	e, _ := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	req := createTestRequest("/v1/sessions", http.MethodPost, "", nil)
	res := performTestRequest(server.Handler, req)

	assert.Equal(http.StatusUnauthorized, res.Code)
}
func TestCreateSession(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	req := createTestRequest("/v1/sessions", http.MethodPost, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	beforeRequest := time.Now().UTC()
	res := performTestRequest(server.Handler, req)
	assert.Equal(http.StatusOK, res.Code)

	var ref dto.Reference
	err := rpc.DecodeJSON(res.Result(), &ref)
	assert.NoError(err)
	assert.NotEmpty(ref.ID)
	assert.Equal("session-manager/session", ref.System)

	repo := repository.NewSessionRepository(e.db)
	session, err := repo.Find(ctx, ref.ID)
	assert.NoError(err)
	assert.Equal(ref.ID, session.ID)
	assert.Equal(models.StatusCreated, session.Status)
	assert.Len(session.Participants, 0)
	assert.True(session.CreatedAt.After(beforeRequest))
	assert.True(session.UpdatedAt.After(beforeRequest))
}

// ---- Test utils ----

func createTestEnv() (*env, context.Context) {
	cfg := config{
		db:             dbutil.SqliteConfig{},
		migrationsPath: "../resources/db/sqlite",
		jwtCredentials: getTestJWTCredentials(),
	}

	db := dbutil.MustConnect(cfg.db)

	err := dbutil.Downgrade(cfg.migrationsPath, cfg.db.Driver(), db)
	if err != nil {
		log.Panic("Failed to apply downgrade migratons", zap.Error(err))
	}

	err = dbutil.Upgrade(cfg.migrationsPath, cfg.db.Driver(), db)
	if err != nil {
		log.Panic("Failed to apply upgrade migratons", zap.Error(err))
	}

	sessionRepo := repository.NewSessionRepository(db)

	e := &env{
		cfg: cfg,
		db:  db,
		sessionService: service.SessionService{
			SessionRepo: sessionRepo,
		},
	}

	return e, context.Background()
}

func performTestRequest(r http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createTestRequest(route, method, role string, body interface{}) *http.Request {
	client := rpc.NewClient(time.Second)
	req, err := client.CreateRequest(method, route, body)
	if err != nil {
		log.Fatal("Failed to create request", zap.Error(err))
	}

	span := opentracing.StartSpan(fmt.Sprintf("%s.%s", method, route))
	opentracing.GlobalTracer().Inject(
		span.Context(),
		opentracing.HTTPHeaders,
		opentracing.HTTPHeadersCarrier(req.Header),
	)

	if role == "" {
		return req
	}

	issuer := jwt.NewIssuer(getTestJWTCredentials())
	token, err := issuer.Issue(jwt.User{
		ID:    "session-manager-user",
		Roles: []string{role},
	}, time.Hour)

	req.Header.Add("Authorization", "Bearer "+token)
	return req
}

func getTestJWTCredentials() jwt.Credentials {
	return jwt.Credentials{
		Issuer: "session-manager-test",
		Secret: "very-secret-secret",
	}
}
