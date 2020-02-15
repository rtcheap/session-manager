package main

import (
	"errors"
	"net/http"

	"github.com/CzarSimon/httputil"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	tracelog "github.com/opentracing/opentracing-go/log"
	"github.com/rtcheap/session-manager/internal/models"
)

const (
	clientIDHeader     = "X-Client-ID"
	clientSecretHeader = "X-Client-Secret"
)

func (e *env) createSession(c *gin.Context) {
	span, ctx := opentracing.StartSpanFromContext(c.Request.Context(), "controller.createSession")
	defer span.Finish()

	creds, httpErr := extractCredentials(c)
	if httpErr != nil {
		span.LogFields(tracelog.Error(httpErr))
		c.Error(httpErr)
		return
	}

	ref, err := e.sessionService.Create(ctx, creds)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, ref)
}

func (e *env) joinSession(c *gin.Context) {
	span, ctx := opentracing.StartSpanFromContext(c.Request.Context(), "controller.joinSession")
	defer span.Finish()

	creds, httpErr := extractCredentials(c)
	if httpErr != nil {
		span.LogFields(tracelog.Error(httpErr))
		c.Error(httpErr)
		return
	}

	offer, err := e.sessionService.Join(ctx, c.Param("sessionId"), creds)
	if err != nil {
		span.LogFields(tracelog.Error(err))
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, offer)
}

func extractCredentials(c *gin.Context) (models.Credentials, *httputil.Error) {
	clientID := c.GetHeader(clientIDHeader)
	clientSecret := c.GetHeader(clientSecretHeader)

	if clientID == "" || clientSecret == "" {
		err := errors.New("clientId or clientSecret is missing")
		return models.Credentials{}, httputil.UnauthorizedError(err)
	}

	return models.Credentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, nil
}
