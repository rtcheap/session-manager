package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/CzarSimon/httputil/dbutil"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/session-manager/internal/models"
)

// SessionRepository persistance interface for sessions.
type SessionRepository interface {
	Find(ctx context.Context, id string) (models.Session, error)
	Save(ctx context.Context, session models.Session) error
	SaveParticipant(ctx context.Context, participant models.Participant) error
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

func (r *sessionRepo) Find(ctx context.Context, id string) (models.Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "session_repo_find")
	defer span.Finish()

	tx, err := r.db.Begin()
	if err != nil {
		err := fmt.Errorf("failed to create database transaction %w", err)
		span.LogFields(tracelog.Error(err))
		return models.Session{}, err
	}
	defer dbutil.Rollback(tx)

	session, err := findSession(ctx, tx, id)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.Session{}, err
	}

	participants, err := findParticipants(ctx, tx, id)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return models.Session{}, err
	}
	session.Participants = participants

	return session, nil
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

func findSession(ctx context.Context, tx *sql.Tx, id string) (models.Session, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "find_session")
	defer span.Finish()

	var s models.Session
	err := tx.QueryRowContext(ctx, findSessionQuery, id).Scan(&s.ID, &s.Status, &s.RelayServer, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		err = fmt.Errorf("failed to query database. %w", err)
		span.LogFields(tracelog.Error(err))
		return models.Session{}, err
	}

	return s, nil
}

const findParticipantsQuery = `
	SELECT 
		id,
		user_id,
		session_id,
		created_at,
		updated_at
	FROM participant
	WHERE
		session_id = ?`

func findParticipants(ctx context.Context, tx *sql.Tx, sessionID string) ([]models.Participant, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "find_session")
	defer span.Finish()

	rows, err := tx.QueryContext(ctx, findParticipantsQuery, sessionID)
	if err != nil {
		err = fmt.Errorf("failed to query for participants %w", err)
		span.LogFields(tracelog.Error(err))
		return nil, err
	}
	defer rows.Close()

	participants := make([]models.Participant, 0)
	for rows.Next() {
		var p models.Participant
		err := rows.Scan(&p.ID, &p.UserID, &p.SessionID, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			err = fmt.Errorf("failed to scan participant %w", err)
			span.LogFields(tracelog.Error(err))
			return nil, err
		}
		participants = append(participants, p)
	}

	return participants, nil
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "session_repo_save")
	defer span.Finish()

	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, insertSessionQuery, &session.ID, &session.Status, &session.RelayServer, now, now)
	if err != nil {
		err = fmt.Errorf("failed to insert row into database. %w", err)
		span.LogFields(tracelog.Error(err))
		return err
	}

	return nil
}

const insertParticipantQuery = `
	INSERT INTO participant(
			id,
			user_id,
			session_id,
			created_at,
			updated_at
		)
	VALUES
		(?, ?, ?, ?, ?)`

func (r *sessionRepo) SaveParticipant(ctx context.Context, p models.Participant) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "session_repo_save_participant")
	defer span.Finish()

	now := getNow()
	_, err := r.db.ExecContext(ctx, insertParticipantQuery, p.ID, p.UserID, p.SessionID, now, now)
	if err != nil {
		err = fmt.Errorf("failed to insert row into database. %w", err)
		span.LogFields(tracelog.Error(err))
		return err
	}

	return nil
}

func getNow() time.Time {
	return time.Now().UTC()
}
