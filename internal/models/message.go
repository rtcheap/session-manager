package models

import (
	"fmt"
)

// Message type
var (
	TypeOffer = "OFFER"
)

// Message websocket message.
type Message struct {
	Type      string      `json:"type,omitempty"`
	SenderID  string      `json:"senderId,omitempty"`
	SessionID string      `json:"sessionId,omitempty"`
	Body      interface{} `json:"body,omitempty"`
}

func (m Message) String() string {
	return fmt.Sprintf("Message(type=%s, senderId=%s, sessionId=%s)", m.Type, m.SenderID, m.SessionID)
}
