package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/id"
	"github.com/CzarSimon/httputil/jwt"
	"github.com/CzarSimon/httputil/logger"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/service-clients/go/serviceregistry"
	"github.com/rtcheap/service-clients/go/turnserver"
	"github.com/rtcheap/session-manager/internal/models"
	"github.com/rtcheap/session-manager/internal/repository"
	"go.uber.org/zap"
)

var log = logger.GetDefaultLogger("session-manager/service")

// Prometheus metrics.
var (
	sessionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "session_created_total",
			Help: "The total number of created sessions",
		},
		[]string{},
	)
	joinsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "session_joins_total",
			Help: "The total number of created sessions",
		},
		[]string{},
	)
)

// SessionOtps session options.
type SessionOtps struct {
	TurnRPCProtocol string
	RelayPort       int
	SessionKey      string
}

// SessionService service to manage sessions.
type SessionService struct {
	Issuer          jwt.Issuer
	TurnRPCProtocol string
	RelayPort       int
	SessionRepo     repository.SessionRepository
	RegistryClient  serviceregistry.Client
	TurnClient      turnserver.Client
}

// Join adds a user as a session participant.
func (s *SessionService) Join(ctx context.Context, sessionID string, creds models.Credentials) (models.SessionOffer, models.Participant, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_join")
	defer span.Finish()

	participant := models.Participant{
		ID:        id.New(),
		UserID:    id.New(),
		SessionID: sessionID,
	}

	svc, err := s.registerParticipant(ctx, participant)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.SessionOffer{}, models.Participant{}, err
	}

	err = s.SessionRepo.SaveParticipant(ctx, participant)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.SessionOffer{}, models.Participant{}, err
	}

	offer, err := s.createOffer(ctx, participant, svc)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.SessionOffer{}, models.Participant{}, err
	}

	joinsTotal.WithLabelValues().Inc()
	return offer, participant, nil
}

// registerParticipant registers a participants to join a session on a turn server.
func (s *SessionService) registerParticipant(ctx context.Context, p models.Participant) (dto.Service, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_register_participant")
	defer span.Finish()

	svc, err := s.findTurnServer(ctx, p.SessionID)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Service{}, err
	}

	userSession := dto.Session{
		UserID: p.UserID,
		Key:    p.SessionID,
	}

	turnURL := fmt.Sprintf("%s://%s:%d", s.TurnRPCProtocol, svc.Location, svc.Port)
	err = s.TurnClient.Register(ctx, turnURL, userSession)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Service{}, httputil.BadGatewayError(err)
	}

	return svc, nil
}

// Create creates a sesssion.
func (s *SessionService) Create(ctx context.Context, creds models.Credentials) (dto.Reference, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_create")
	defer span.Finish()

	session, err := s.assignSessionToTurnServer(ctx)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Reference{}, err
	}

	err = s.SessionRepo.Save(ctx, session)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Reference{}, err
	}

	sessionsTotal.WithLabelValues().Inc()
	return dto.Reference{ID: session.ID, System: "session-manager/session"}, nil
}

func (s *SessionService) assignSessionToTurnServer(ctx context.Context) (models.Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_assign_session_to_turn_server")
	defer span.Finish()

	services, err := s.RegistryClient.FindByApplication(ctx, "turn-server", true)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.Session{}, httputil.BadGatewayError(err)
	}

	best, err := s.findBestTurnServer(ctx, services)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.Session{}, err
	}

	return models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: best.ID,
	}, nil
}

func (s *SessionService) findBestTurnServer(ctx context.Context, services []dto.Service) (dto.Service, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_find_best_turn_server")
	defer span.Finish()

	connections := make([]uint64, len(services))
	wg := sync.WaitGroup{}

	for i := range services {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			svc := services[idx]
			url := fmt.Sprintf("%s://%s:%d", s.TurnRPCProtocol, svc.Location, svc.Port)
			stats, err := s.TurnClient.GetStatistics(ctx, url)
			if err != nil {
				log.Warn("failed to gather statistics from "+url, zap.Error(err))
				span.LogFields(tracelog.String("candidate", url), tracelog.Error(err))
				connections[idx] = math.MaxUint64
			} else {
				connections[idx] = stats.InProgress()
			}
		}(i)
	}
	wg.Wait()

	best, err := leastConnTurnServer(services, connections)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Service{}, err
	}

	return best, nil
}

func leastConnTurnServer(services []dto.Service, connections []uint64) (dto.Service, error) {
	var best dto.Service
	var least uint64 = math.MaxUint64
	for i, conns := range connections {
		if conns < least {
			least = conns
			best = services[i]
		}
	}

	if best.ID == "" {
		err := httputil.InternalServerError(errors.New("no turn-server found"))
		return dto.Service{}, err
	}

	return best, nil
}

// createOffer creates an ice offer for session participants to use to connect.
func (s *SessionService) createOffer(ctx context.Context, p models.Participant, svc dto.Service) (models.SessionOffer, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_create_offer")
	defer span.Finish()

	jwtUser := jwt.User{
		ID:    p.UserID,
		Roles: []string{"USER"},
	}

	token, err := s.Issuer.Issue(jwtUser, 24*time.Hour)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.SessionOffer{}, err
	}

	offer := models.SessionOffer{
		Token: token,
		TURN: models.TurnCandidate{
			URL:      fmt.Sprintf("turn:%s:%d", svc.Location, s.RelayPort),
			Username: p.UserID,
		},
		STUN: models.StunCandidate{
			URL: fmt.Sprintf("stun:%s:%d", svc.Location, s.RelayPort),
		},
	}

	return offer, nil
}

func (s *SessionService) findTurnServer(ctx context.Context, sessionID string) (dto.Service, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_session_service_find_turn_server")
	defer span.Finish()

	session, err := s.SessionRepo.Find(ctx, sessionID)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Service{}, httputil.PreconditionRequiredError(err)
	}

	svc, err := s.RegistryClient.Find(ctx, session.RelayServer)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return dto.Service{}, httputil.BadGatewayError(err)
	}

	return svc, nil
}
