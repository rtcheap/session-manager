package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/gorilla/websocket"
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

	svc1ID := id.New()
	svc2ID := id.New()
	svc3ID := id.New()
	mockRegistryClient := MockClient("http://service-registry:8080", jwt.SystemRole, map[string]rpc.MockResponse{
		"GET:http://service-registry:8080/v1/services?application=turn-server&only-healthy=true": rpc.MockResponse{
			Body: []dto.Service{
				dto.Service{
					ID:          svc1ID,
					Application: "turn-server",
					Location:    "turn-1",
					Port:        8081,
					Status:      dto.StatusHealty,
				},
				dto.Service{
					ID:          svc2ID,
					Application: "turn-server",
					Location:    "turn-2",
					Port:        8080,
					Status:      dto.StatusHealty,
				},
				dto.Service{
					ID:          svc3ID,
					Application: "turn-server",
					Location:    "turn-3",
					Port:        8080,
					Status:      dto.StatusHealty,
				},
			},
			Err: nil,
		},
	})

	e.sessionService.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	mockTurnClient := MockClient("", jwt.SystemRole, map[string]rpc.MockResponse{
		"GET:http://turn-1:8081/v1/sessions/statistics": rpc.MockResponse{
			Body: dto.SessionStatistics{
				Started: 150,
				Ended:   50,
			},
			Err: nil,
		},
		"GET:http://turn-2:8080/v1/sessions/statistics": rpc.MockResponse{
			Body: dto.SessionStatistics{
				Started: 100,
				Ended:   50,
			},
			Err: nil,
		},
		"GET:http://turn-3:8080/v1/sessions/statistics": rpc.MockResponse{
			Body: nil,
			Err:  httputil.ServiceUnavailableError(nil),
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
	assert.Equal(session.RelayServer, svc2ID)
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

	req := createTestRequest("/v1/sessions/"+session.ID, http.MethodGet, "", nil)
	res := performTestRequest(server.Handler, req)

	assert.Equal(http.StatusUnauthorized, res.Code)
}

func TestJoinSession_NoSession(t *testing.T) {
	assert := assert.New(t)
	e, _ := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	sessionID := id.New()
	req := createTestRequest("/v1/sessions/"+sessionID, http.MethodGet, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	res := performTestRequest(server.Handler, req)

	assert.Equal(http.StatusPreconditionRequired, res.Code)
}

func TestJoinSession_BadGateway_TurnServer(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	type statusRes struct {
		Status string `json:"status,omitempty"`
	}

	turnID := id.New()
	mockRegistryClient := MockClient("http://service-registry:8080", jwt.SystemRole, map[string]rpc.MockResponse{
		"GET:http://service-registry:8080/v1/services/" + turnID: rpc.MockResponse{
			Body: dto.Service{
				ID:          turnID,
				Application: "turn-server",
				Location:    "assigned-turn",
				Port:        8083,
				Status:      dto.StatusHealty,
			},
			Err: nil,
		},
	})

	e.sessionService.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	mockTurnClient := MockClient("", jwt.SystemRole, map[string]rpc.MockResponse{
		"POST:http://assigned-turn:8083/v1/sessions": rpc.MockResponse{
			Err: httputil.InternalServerError(nil),
		},
	})

	e.sessionService.TurnClient = turnserver.NewClient(mockTurnClient)

	repo := repository.NewSessionRepository(e.db)
	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: turnID,
	}

	err := repo.Save(ctx, session)
	assert.NoError(err)

	stored, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(stored.Participants, 0)

	req := createTestRequest("/v1/sessions/"+session.ID, http.MethodGet, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	res := performTestRequest(server.Handler, req)
	assert.Equal(http.StatusBadGateway, res.Code)

	unchanged, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(unchanged.Participants, 0)
}

func TestJoinSession_BadGateway_ServiceRegistry(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	type statusRes struct {
		Status string `json:"status,omitempty"`
	}

	turnID := id.New()
	mockRegistryClient := MockClient("http://service-registry:8080", jwt.SystemRole, map[string]rpc.MockResponse{
		"GET:http://service-registry:8080/v1/services/" + turnID: rpc.MockResponse{
			Err: httputil.InternalServerError(nil),
		},
	})

	e.sessionService.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	repo := repository.NewSessionRepository(e.db)
	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: turnID,
	}

	err := repo.Save(ctx, session)
	assert.NoError(err)

	stored, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(stored.Participants, 0)

	req := createTestRequest("/v1/sessions/"+session.ID, http.MethodGet, "", nil)
	req.Header.Add(clientIDHeader, id.New())
	req.Header.Add(clientSecretHeader, id.New())
	res := performTestRequest(server.Handler, req)
	assert.Equal(http.StatusBadGateway, res.Code)

	unchanged, err := repo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(unchanged.Participants, 0)
}

func TestJoinSession(t *testing.T) {
	assert := assert.New(t)
	e, ctx := createTestEnv()
	defer e.db.Close()
	server := newServer(e)

	type statusRes struct {
		Status string `json:"status,omitempty"`
	}

	turnID := id.New()
	mockRegistryClient := MockClient("http://service-registry:8080", jwt.SystemRole, map[string]rpc.MockResponse{
		"GET:http://service-registry:8080/v1/services/" + turnID: rpc.MockResponse{
			Body: dto.Service{
				ID:          turnID,
				Application: "turn-server",
				Location:    "assigned-turn",
				Port:        8083,
				Status:      dto.StatusHealty,
			},
			Err: nil,
		},
	})

	e.sessionService.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	mockTurnClient := MockClient("", jwt.SystemRole, map[string]rpc.MockResponse{
		"POST:http://assigned-turn:8083/v1/sessions": rpc.MockResponse{
			Body: statusRes{Status: "OK"},
			Err:  nil,
		},
	})

	e.sessionService.TurnClient = turnserver.NewClient(mockTurnClient)

	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: turnID,
	}

	err := e.sessionService.SessionRepo.Save(ctx, session)
	assert.NoError(err)

	stored, err := e.sessionService.SessionRepo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(stored.Participants, 0)

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			log.Error("unexpected error closing server", zap.Error(err))
		}
	}()
	time.Sleep(50 * time.Millisecond)

	headers := http.Header{}
	headers.Add(clientIDHeader, id.New())
	headers.Add(clientSecretHeader, id.New())

	url := fmt.Sprintf("ws://127.0.0.1:%s/v1/sessions/%s", e.cfg.port, session.ID)
	conn, res, err := websocket.DefaultDialer.Dial(url, headers)
	assert.NoError(err)
	if err == nil {
		assert.Equal(http.StatusSwitchingProtocols, res.StatusCode)
		defer conn.Close()
		for {
			mt, data, err := conn.ReadMessage()
			assert.NoError(err)
			assert.Equal(websocket.TextMessage, mt)

			var message models.Message
			err = json.Unmarshal(data, &message)
			assert.NoError(err)
			assert.Equal(models.TypeOffer, message.Type)
			assert.Equal(session.ID, message.SessionID)

			break
		}

	}

	time.Sleep(50 * time.Millisecond)
	changed, err := e.sessionService.SessionRepo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(changed.Participants, 1)
}

// ---- Test utils ----

func createTestEnv() (*env, context.Context) {
	cfg := config{
		port: "34547",
		db:   dbutil.SqliteConfig{},
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
	sessionService := &service.SessionService{
		Issuer:          jwt.NewIssuer(cfg.jwtCredentials),
		TurnRPCProtocol: cfg.turn.rpcProtocol,
		RelayPort:       cfg.turn.udpPort,
		SessionRepo:     sessionRepo,
	}

	messageService := &service.MessageService{
		Socket: service.NewWebsocketHandler(),
	}

	e := &env{
		cfg:            cfg,
		db:             db,
		sessionService: sessionService,
		messageService: messageService,
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

func MockClient(baseURL, role string, reponses map[string]rpc.MockResponse) client.Client {
	c := client.Client{
		RPCClient: &rpc.MockClient{
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
