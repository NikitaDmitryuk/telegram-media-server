package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/google/uuid"
)

const (
	webhookTimeout    = 10 * time.Second
	webhookRetryWait  = 2 * time.Second
	webhookMaxRetries = 2
)

// WebhookPayload is sent to TMS_WEBHOOK_URL on download completion/failure.
type WebhookPayload struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"` // completed, failed, stopped
	Error   string `json:"error,omitempty"`
	EventID string `json:"event_id"`
}

// SendCompletionWebhook POSTs the payload to webhookURL asynchronously (best effort, with retries).
func SendCompletionWebhook(webhookURL string, movieID uint, title, status, errMsg string) {
	if webhookURL == "" {
		return
	}
	eventID := uuid.New().String()
	payload := WebhookPayload{
		ID:      movieID,
		Title:   title,
		Status:  status,
		Error:   errMsg,
		EventID: eventID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Webhook: failed to marshal payload")
		return
	}
	go doSendWithRetry(webhookURL, body, movieID, eventID)
}

func doSendWithRetry(webhookURL string, body []byte, movieID uint, eventID string) {
	client := &http.Client{Timeout: webhookTimeout}
	for attempt := 0; attempt <= webhookMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(webhookRetryWait)
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			logutils.Log.WithError(err).
				WithFields(map[string]any{"movie_id": movieID, "event_id": eventID}).
				Warn("Webhook: failed to create request")
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			logutils.Log.WithError(err).
				WithFields(map[string]any{"movie_id": movieID, "event_id": eventID, "attempt": attempt + 1}).
				Warn("Webhook: request failed")
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logutils.Log.WithFields(map[string]any{"movie_id": movieID, "event_id": eventID}).Debug("Webhook delivered")
			return
		}
		logutils.Log.WithFields(map[string]any{
			"movie_id": movieID, "event_id": eventID, "status": resp.StatusCode, "attempt": attempt + 1,
		}).Warn("Webhook: non-2xx response")
	}
	logutils.Log.WithFields(map[string]any{
		"movie_id": movieID, "event_id": eventID, "attempts": webhookMaxRetries + 1,
	}).Warn("Webhook: failed to deliver after all retries")
}
