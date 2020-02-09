package service

import (
	"context"

	"github.com/CzarSimon/httputil/id"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/dto"
	"github.com/rtcheap/session-manager/internal/models"
	"github.com/rtcheap/session-manager/internal/repository"
)

// SessionService service to manage sessions.
type SessionService struct {
	SessionRepo repository.SessionRepository
}

// Create creates a sesssion.
func (s *SessionService) Create(ctx context.Context, creds models.Credentials) (dto.Reference, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service.SessionService.Create")
	defer span.Finish()

	session := models.Session{
		ID:          id.New(),
		Status:      models.StatusCreated,
		RelayServer: "",
	}

	err := s.SessionRepo.Save(ctx, session)
	if err != nil {
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return dto.Reference{}, err
	}

	ref := dto.Reference{
		ID:     session.ID,
		System: "session-manager/session",
	}

	span.LogFields(tracelog.Bool("success", true))
	return ref, nil
}
