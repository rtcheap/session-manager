package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/session-manager/internal/models"
)

// SessionRepository persistance interface for sessions.
type SessionRepository interface {
	Find(ctx context.Context, id string) (models.Session, error)
	Save(ctx context.Context, session models.Session) error
}

// NewSessionRepository creates a new SQL SessionRepository.
func NewSessionRepository(db *sql.DB) SessionRepository {
	return &sessionRepo{
		db: db,
	}
}

type sessionRepo struct {
	db *sql.DB
}

const findSessionQuery = `
	SELECT 
		id, 
		status, 
		relay_server, 
		created_at, 
		updated_at 
	FROM session
	WHERE 
		id = ?`

func (r *sessionRepo) Find(ctx context.Context, id string) (models.Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "sessionRepo.Find")
	defer span.Finish()

	var s models.Session
	err := r.db.QueryRowContext(ctx, findSessionQuery, id).Scan(&s.ID, &s.Status, &s.RelayServer, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		err = fmt.Errorf("failed to query database. %w", err)
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return models.Session{}, err
	}

	span.LogFields(tracelog.Bool("success", true))
	return s, nil
}

const insertSessionQuery = `
	INSERT INTO session(
			id,
			status,
			relay_server,
			created_at,
			updated_at
		)
	VALUES
		(?, ?, ?, ?, ?)`

func (r *sessionRepo) Save(ctx context.Context, session models.Session) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "sessionRepo.Save")
	defer span.Finish()

	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, insertSessionQuery, &session.ID, &session.Status, &session.RelayServer, now, now)
	if err != nil {
		err = fmt.Errorf("failed to insert row into database. %w", err)
		span.LogFields(tracelog.Bool("success", false), tracelog.Error(err))
		return err
	}

	span.LogFields(tracelog.Bool("success", true))
	return nil
}
