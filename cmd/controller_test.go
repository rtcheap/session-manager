package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/client"
	"github.com/CzarSimon/httputil/client/rpc"
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/id"
	"github.com/CzarSimon/httputil/jwt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/opentracing/opentracing-go"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/service-clients/go/serviceregistry"
	"github.com/rtcheap/service-clients/go/turnserver"
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

	mockRegistryClient := MockClient("http://service-registry:8080", jwt.SystemRole, map[string]mockResponse{
		"GET:http://service-registry:8080/v1/services?application=turn-server&only-healthy=true": mockResponse{
			body: []dto.Service{
				dto.Service{
					ID:          id.New(),
					Application: "turn-server",
					Location:    "turn-1",
					Port:        8080,
					Status:      dto.StatusHealty,
				},
				dto.Service{
					ID:          id.New(),
					Application: "turn-server",
					Location:    "turn-2",
					Port:        8080,
					Status:      dto.StatusHealty,
				},
				dto.Service{
					ID:          id.New(),
					Application: "turn-server",
					Location:    "turn-3",
					Port:        8080,
					Status:      dto.StatusHealty,
				},
			},
			err: nil,
		},
	})

	e.sessionService.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	mockTurnClient := MockClient("", jwt.SystemRole, map[string]mockResponse{
		"GET:http://turn-1:8080/v1/sessions/statistics": mockResponse{
			body: dto.SessionStatistics{
				Started: 150,
				Ended:   50,
			},
			err: nil,
		},
		"GET:http://turn-2:8080/v1/sessions/statistics": mockResponse{
			body: dto.SessionStatistics{
				Started: 100,
				Ended:   50,
			},
			err: nil,
		},
		"GET:http://turn-3:8080/v1/sessions/statistics": mockResponse{
			body: nil,
			err:  httputil.ServiceUnavailableError(nil),
		},
	})

	e.sessionService.TurnClient = turnserver.NewClient(mockTurnClient)

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
	assert.Equal(session.RelayServer, "turn-2:3478")
	assert.True(session.CreatedAt.After(beforeRequest))
	assert.True(session.UpdatedAt.After(beforeRequest))
}

func TestJoinSession_NoCredentials(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: "turn-2:3478",
	}
	err := repository.NewSessionRepository(e.db).Save(ctx, session)
	assert.NoError(err)

	req := createTestRequest("/v1/sessions/"+session.ID, http.MethodPut, "", nil)
	res := performTestRequest(server.Handler, req)

	assert.Equal(http.StatusUnauthorized, res.Code)
}

func TestJoinSession_NoSession(t *testing.T) {
	assert := assert.New(t)
	e, _ := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	sessionID := id.New()
	req := createTestRequest("/v1/sessions/"+sessionID, http.MethodPut, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	res := performTestRequest(server.Handler, req)

	assert.Equal(http.StatusPreconditionRequired, res.Code)
}

func TestJoinSession(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	repo := repository.NewSessionRepository(e.db)
	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: "turn-2:3478",
	}

	err := repo.Save(ctx, session)
	assert.NoError(err)

	stored, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(stored.Participants, 0)

	req := createTestRequest("/v1/sessions/"+session.ID, http.MethodPut, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	performTestRequest(server.Handler, req)

	time.Sleep(100 * time.Millisecond)
	changed, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(changed.Participants, 1)
}

// ---- Test utils ----

func createTestEnv() (*env, context.Context) {
	cfg := config{
		db: dbutil.SqliteConfig{},
		turn: turnConfig{
			udpPort:     3478,
			rpcProtocol: "http",
		},
		migrationsPath: "../resources/db/sqlite",
		jwtCredentials: getTestJWTCredentials(),
	}

	db := dbutil.MustConnect(cfg.db)

	_, err := db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Panic("Failed to enable foreign_keys", zap.Error(err))
	}

	err = dbutil.Downgrade(cfg.migrationsPath, cfg.db.Driver(), db)
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
			TurnRPCProtocol: cfg.turn.rpcProtocol,
			RelayPort:       cfg.turn.udpPort,
			SessionRepo:     sessionRepo,
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

type mockRPCClient struct {
	rpc.Client
	Responses map[string]mockResponse
}

func MockClient(baseURL, role string, reponses map[string]mockResponse) client.Client {
	c := client.Client{
		RPCClient: &mockRPCClient{
			Client:    rpc.NewClient(time.Second),
			Responses: reponses,
		},
		Issuer:    jwt.NewIssuer(getTestJWTCredentials()),
		BaseURL:   baseURL,
		Role:      role,
		UserAgent: "mockClient",
	}

	return c
}

type mockResponse struct {
	body interface{}
	err  error
}

func (c *mockRPCClient) Do(req *http.Request) (*http.Response, error) {
	time.Sleep(10 * time.Millisecond)
	key := fmt.Sprintf("%s:%s", req.Method, req.URL)
	mockRes, ok := c.Responses[key]
	if !ok {
		err := fmt.Errorf("could not find uri %s", key)
		return nil, httputil.NotFoundError(err)
	}

	if mockRes.err != nil {
		return nil, mockRes.err
	}

	var body io.ReadCloser
	headers := http.Header{}
	if mockRes.body != nil {
		bytesBody, err := json.Marshal(mockRes.body)
		if err != nil {
			return nil, err
		}
		body = ioutil.NopCloser(bytes.NewBuffer(bytesBody))
		headers.Add("Content-Type", "application/json")
	} else {
		body = http.NoBody
	}

	status := http.StatusOK
	res := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     fmt.Sprintf("%d - %s", status, http.StatusText(status)),
		StatusCode: status,
		Body:       body,
		Header:     headers,
	}

	return res, nil
}
