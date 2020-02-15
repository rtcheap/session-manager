package models

import (
	"fmt"
	"time"
)

// Session statuses.
const (
	StatusCreated = "CREATED"
	StatusStarted = "STARTED"
	StatusEnded   = "ENDED"
)

// Session represents communication between many parties.
type Session struct {
	ID           string
	Status       string
	RelayServer  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Participants []Participant
}

func (s Session) String() string {
	return fmt.Sprintf(
		"Session(id=%s, status=%s, relayServer=%s, createdAt=%v, updatedAt=%v, participants=%d)",
		s.ID,
		s.Status,
		s.RelayServer,
		s.CreatedAt,
		s.UpdatedAt,
		len(s.Participants),
	)
}

// Participant member of a session.
type Participant struct {
	ID        string
	UserID    string
	SessionID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p Participant) String() string {
	return fmt.Sprintf(
		"Participant(id=%s, userId=%s, sessionId=%s, createdAt=%v, updatedAt=%v)",
		p.ID,
		p.UserID,
		p.SessionID,
		p.CreatedAt,
		p.UpdatedAt,
	)
}

// SessionOffer metadata required for creating a peer-to-peer session.
type SessionOffer struct {
	Token string        `json:"token,omitempty"`
	TRUN  TurnCandidate `json:"trun,omitempty"`
	STUN  StunCandidate `json:"stun,omitempty"`
}

// TurnCandidate ICE candidate for inititating a peer connection usring a relay server.
type TurnCandidate struct {
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
}

// StunCandidate ICE candidate for inititating a peer connection using a STUN server for network information exchange.
type StunCandidate struct {
	URL string `json:"url,omitempty"`
}
