package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/CzarSimon/httputil"
	"github.com/CzarSimon/httputil/id"
	"github.com/CzarSimon/httputil/logger"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/service-clients/go/serviceregistry"
	"github.com/rtcheap/service-clients/go/turnserver"
	"github.com/rtcheap/session-manager/internal/models"
	"github.com/rtcheap/session-manager/internal/repository"
	"go.uber.org/zap"
)

var log = logger.GetDefaultLogger("session-manager/service")

// SessionService service to manage sessions.
type SessionService struct {
	TurnRPCProtocol string
	RelayPort       int
	SessionRepo     repository.SessionRepository
	RegistryClient  serviceregistry.Client
	TurnClient      turnserver.Client
}

// Join adds a user as a session participant.
func (s *SessionService) Join(ctx context.Context, sessionID string, creds models.Credentials) (models.SessionOffer, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service.SessionService.Join")
	defer span.Finish()

	participant := models.Participant{
		ID:        id.New(),
		UserID:    id.New(),
		SessionID: sessionID,
	}

	// TODO: Register participant

	err := s.SessionRepo.SaveParticipant(ctx, participant)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.SessionOffer{}, httputil.PreconditionRequiredError(err)
	}

	// TODO: create offer

	return models.SessionOffer{}, nil
}

// Create creates a sesssion.
func (s *SessionService) Create(ctx context.Context, creds models.Credentials) (dto.Reference, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service.SessionService.Create")
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

	return dto.Reference{ID: session.ID, System: "session-manager/session"}, nil
}

func (s *SessionService) assignSessionToTurnServer(ctx context.Context) (models.Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service.SessionService.assignSessionToTurnServer")
	defer span.Finish()

	services, err := s.RegistryClient.FindByApplication(ctx, "turn-server", true)
	if err != nil {
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return models.Session{}, httputil.BadGatewayError(err)
	}

	best, err := s.findBestTurnServer(ctx, services)
	if err != nil {
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return models.Session{}, err
	}

	span.LogFields(tracelog.Bool("success", true))
	return models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: fmt.Sprintf("%s:%d", best.Location, s.RelayPort),
	}, nil
}

func (s *SessionService) findBestTurnServer(ctx context.Context, services []dto.Service) (dto.Service, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service.SessionService.findBestTurnServer")
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
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return dto.Service{}, err
	}

	span.LogFields(tracelog.Bool("success", true))
	return best, nil
}
