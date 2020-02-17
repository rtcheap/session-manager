package service

import (
	"context"
	"net/http"

	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rtcheap/session-manager/internal/models"
)

// Prometheus metrics.
var (
	messagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "messages_sent_total",
			Help: "The total number of send message",
		},
		[]string{"type"},
	)
)

// MessageService service to create and send messages.
type MessageService struct {
	Socket         *WebsocketHandler
	SessionService *SessionService
}

// ConnectAndSend connects to a socket channel and sends the provided message over it.
func (m *MessageService) ConnectAndSend(
	ctx context.Context,
	sessionID string,
	creds models.Credentials,
	r *http.Request,
	w http.ResponseWriter,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_message_service_connect_and_send")
	defer span.Finish()

	offer, participant, err := m.SessionService.Join(ctx, sessionID, creds)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return err
	}

	err = m.Socket.Connect(ctx, participant, r, w)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return err
	}

	message := models.Message{
		Type:      models.TypeOffer,
		SessionID: sessionID,
		Body:      offer,
	}

	err = m.sendMessage(ctx, message)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return err
	}

	return nil
}

// Send sends a message to a connected socket.
func (m *MessageService) Send(ctx context.Context, message models.Message) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_message_service_send")
	defer span.Finish()

	err := m.sendMessage(ctx, message)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return err
	}

	return nil
}

func (m *MessageService) sendMessage(ctx context.Context, message models.Message) error {
	err := m.Socket.Send(ctx, message)
	if err != nil {
		return nil
	}

	messagesTotal.WithLabelValues(message.Type).Inc()
	return nil
}
