package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/CzarSimon/httputil"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/session-manager/internal/models"
	"go.uber.org/zap"
)

type client struct {
	id   string
	send chan []byte
}

// channel bla bla
type channel struct {
	mu      sync.RWMutex
	clients map[string]*client
}

func (ch *channel) join(connID string) (*client, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	_, ok := ch.clients[connID]
	if ok {
		err := fmt.Errorf("clients(userId=%s) has already joined channel", connID)
		return nil, httputil.ConflictError(err)
	}

	sendChan := make(chan []byte)
	c := &client{
		id:   connID,
		send: sendChan,
	}

	ch.clients[connID] = c
	return c, nil
}

// WebsocketHandler relayer of websocket messages.
type WebsocketHandler struct {
	upgrader *websocket.Upgrader
	mu       sync.RWMutex
	channels map[string]*channel
}

// NewWebsocketHandler creates a new WebsocketHandler.
func NewWebsocketHandler() *WebsocketHandler {
	return &WebsocketHandler{
		upgrader: &websocket.Upgrader{},
		mu:       sync.RWMutex{},
		channels: make(map[string]*channel),
	}
}

// Connect connects a participant to a websocket channel
func (h *WebsocketHandler) Connect(ctx context.Context, p models.Participant, r *http.Request, w http.ResponseWriter) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "service_websocket_handler_connect")
	defer span.Finish()

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		err = fmt.Errorf("failed to upgrade connetion to a websocket %w", err)
		span.LogFields(tracelog.Error(err))
		return err
	}

	ch := h.findOrCreateChannel(p.SessionID)
	c, err := ch.join(p.UserID)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		return err
	}

	go registerSocketReciever(c, ws)
	return nil
}

// Send sends a message to all clients connected to the channel.
func (h *WebsocketHandler) Send(ctx context.Context, message models.Message) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "service_websocket_handler_send")
	defer span.Finish()

	ch, ok := h.findChannel(message.SessionID)
	if !ok {
		err := fmt.Errorf("no such channel %s", message.SessionID)
		err = httputil.PreconditionRequiredError(err)
		span.LogFields(tracelog.Error(err))
		return err
	}

	err := sendToChannel(ctx, ch, message)
	if !ok {
		span.LogFields(tracelog.Error(err))
		return err
	}

	return nil
}

func sendToChannel(ctx context.Context, ch *channel, message models.Message) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "service_send_to_channel")
	defer span.Finish()

	data, err := json.Marshal(message)
	if err != nil {
		err = fmt.Errorf("failed to serialize json %w", err)
		span.LogFields(tracelog.Error(err))
		return err
	}

	ch.mu.RLock()
	for _, client := range ch.clients {
		if client.id == message.SenderID {
			continue
		}

		client.send <- data
	}
	ch.mu.RUnlock()

	return nil
}

func (h *WebsocketHandler) findChannel(chanID string) (*channel, bool) {
	h.mu.RLock()
	ch, ok := h.channels[chanID]
	h.mu.RUnlock()

	return ch, ok
}

func (h *WebsocketHandler) findOrCreateChannel(chanID string) *channel {
	ch, ok := h.findChannel(chanID)

	if !ok {
		ch = &channel{
			mu:      sync.RWMutex{},
			clients: make(map[string]*client),
		}
		h.mu.Lock()
		h.channels[chanID] = ch
		h.mu.Unlock()
	}

	return ch
}

func registerSocketReciever(c *client, ws *websocket.Conn) {
	for {
		select {
		case data, ok := <-c.send:
			if !ok {
				closeSocket(ws)
				return
			}

			writeMessage(ws, websocket.TextMessage, data)
		}
	}
}

func closeSocket(ws *websocket.Conn) {
	writeMessage(ws, websocket.CloseMessage, []byte{})
	err := ws.Close()
	if err != nil {
		log.Warn("failed to close websocked connection", zap.Error(err))
	}
}

func writeMessage(ws *websocket.Conn, messageType int, data []byte) {
	err := ws.WriteMessage(messageType, data)
	if err != nil {
		log.Warn("failed to send message", zap.Error(err))
	}
}
