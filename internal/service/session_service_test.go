package service_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/CzarSimon/httputil/client"
	"github.com/CzarSimon/httputil/client/rpc"
	"github.com/CzarSimon/httputil/dbutil"
	"github.com/CzarSimon/httputil/id"
	"github.com/CzarSimon/httputil/jwt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/service-clients/go/serviceregistry"
	"github.com/rtcheap/service-clients/go/turnserver"
	"github.com/rtcheap/session-manager/internal/models"
	"github.com/rtcheap/session-manager/internal/repository"
	"github.com/rtcheap/session-manager/internal/service"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestJoinSession(t *testing.T) {
	assert := assert.New(t)
	s, ctx := createService()

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

	s.RegistryClient = serviceregistry.NewClient(mockRegistryClient)

	mockTurnClient := MockClient("", jwt.SystemRole, map[string]rpc.MockResponse{
		"POST:http://assigned-turn:8083/v1/sessions": rpc.MockResponse{
			Body: statusRes{Status: "OK"},
			Err:  nil,
		},
	})

	s.TurnClient = turnserver.NewClient(mockTurnClient)

	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: turnID,
	}

	err := s.SessionRepo.Save(ctx, session)
	assert.NoError(err)

	stored, err := s.SessionRepo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(stored.Participants, 0)

	creds := models.Credentials{
		ClientID:     id.New(),
		ClientSecret: id.New(),
	}
	offer, participant, err := s.Join(ctx, session.ID, creds)
	assert.NoError(err)
	assert.Equal("turn:assigned-turn:3478", offer.TURN.URL)
	assert.Equal("stun:assigned-turn:3478", offer.STUN.URL)
	assert.NotEmpty(offer.Token)

	verifier := jwt.NewVerifier(getTestJWTCredentials(), 0)
	user, err := verifier.Verify(offer.Token)
	assert.NoError(err)
	assert.True(user.HasRole("USER"))
	assert.Len(user.Roles, 1)

	changed, err := s.SessionRepo.Find(ctx, session.ID)
	assert.NoError(err)
	assert.Len(changed.Participants, 1)
	assert.Equal(changed.Participants[0].UserID, user.ID)
	assert.Equal(changed.Participants[0].ID, participant.ID)
}

func createService() (service.SessionService, context.Context) {
	dbConf := dbutil.SqliteConfig{}
	migrationsPath := "../../resources/db/sqlite"
	db := dbutil.MustConnect(dbConf)

	_, err := db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Panic("Failed to enable foreign_keys", zap.Error(err))
	}

	err = dbutil.Downgrade(migrationsPath, dbConf.Driver(), db)
	if err != nil {
		log.Panic("Failed to apply downgrade migratons", zap.Error(err))
	}

	err = dbutil.Upgrade(migrationsPath, dbConf.Driver(), db)
	if err != nil {
		log.Panic("Failed to apply upgrade migratons", zap.Error(err))
	}

	s := service.SessionService{
		Issuer:          jwt.NewIssuer(getTestJWTCredentials()),
		TurnRPCProtocol: "http",
		RelayPort:       3478,
		SessionRepo:     repository.NewSessionRepository(db),
	}

	return s, context.Background()
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
