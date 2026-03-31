package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/google/uuid"
)

const (
	webhookTimeout      = 10 * time.Second
	webhookRetryWait    = 2 * time.Second
	webhookMaxRetries   = 2
	webhookBodySniff    = 512
	formatTMS           = "tms"
	formatOpenClawWake  = "openclaw_wake"
	formatOpenClawAgent = "openclaw_agent"

	statusCompleted = "completed"
	statusFailed    = "failed"
	statusStopped   = "stopped"
)

// WebhookPayload is the default JSON body (TMS_WEBHOOK_FORMAT=json or tms, or custom OpenClaw mappings).
type WebhookPayload struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"` // completed, failed, stopped
	Error   string `json:"error,omitempty"`
	EventID string `json:"event_id"`
}

type openclawWakeBody struct {
	Text string `json:"text"`
	Mode string `json:"mode,omitempty"`
}

type openclawAgentBody struct {
	Message  string `json:"message"`
	Name     string `json:"name,omitempty"`
	WakeMode string `json:"wakeMode,omitempty"`
}

// effectiveWebhookFormat picks how to encode the webhook body.
// OpenClaw /hooks/wake requires {"text":"..."}; /hooks/agent requires {"message":"..."}.
// Without hooks.mappings, posting TMS {id,title,...} to /hooks/wake returns 400.
func effectiveWebhookFormat(webhookURL, explicit string) string {
	switch strings.ToLower(strings.TrimSpace(explicit)) {
	case "openclaw_wake", "wake":
		return formatOpenClawWake
	case "openclaw_agent", "agent":
		return formatOpenClawAgent
	case "tms", "json":
		return formatTMS
	}
	if strings.TrimSpace(explicit) != "" {
		return formatTMS
	}
	u := strings.ToLower(webhookURL)
	if strings.Contains(u, "/hooks/wake") {
		return formatOpenClawWake
	}
	if strings.Contains(u, "/hooks/agent") {
		return formatOpenClawAgent
	}
	return formatTMS
}

func openClawEventText(movieID uint, title, status, errMsg string) string {
	switch status {
	case statusCompleted:
		return fmt.Sprintf("Telegram Media Server: download completed — %q (library id %d)", title, movieID)
	case statusFailed:
		if errMsg != "" {
			return fmt.Sprintf("Telegram Media Server: download failed — %q: %s (library id %d)", title, errMsg, movieID)
		}
		return fmt.Sprintf("Telegram Media Server: download failed — %q (library id %d)", title, movieID)
	case statusStopped:
		return fmt.Sprintf("Telegram Media Server: download stopped — %q (library id %d)", title, movieID)
	default:
		return fmt.Sprintf("Telegram Media Server: event %q — %q (library id %d)", status, title, movieID)
	}
}

func marshalWebhookBody(format string, movieID uint, title, status, errMsg, eventID string) ([]byte, error) {
	safeTitle := strings.ToValidUTF8(title, "\uFFFD")
	safeErr := strings.ToValidUTF8(errMsg, "\uFFFD")

	switch format {
	case formatOpenClawWake:
		return json.Marshal(openclawWakeBody{
			Text: openClawEventText(movieID, safeTitle, status, safeErr),
			Mode: "now",
		})
	case formatOpenClawAgent:
		return json.Marshal(openclawAgentBody{
			Message:  openClawEventText(movieID, safeTitle, status, safeErr),
			Name:     "TMS",
			WakeMode: "now",
		})
	default:
		payload := WebhookPayload{
			ID:      movieID,
			Title:   safeTitle,
			Status:  status,
			Error:   safeErr,
			EventID: eventID,
		}
		return json.Marshal(payload)
	}
}

// SendCompletionWebhook POSTs to webhookURL asynchronously (best effort, with retries).
// formatEnv is TMS_WEBHOOK_FORMAT; empty triggers auto-detect from URL (/hooks/wake → OpenClaw wake JSON).
// If authToken is non-empty, sends Authorization: Bearer <token> (OpenClaw also accepts x-openclaw-token; we use Bearer).
func SendCompletionWebhook(webhookURL, authToken, formatEnv string, movieID uint, title, status, errMsg string) {
	if webhookURL == "" {
		return
	}
	format := effectiveWebhookFormat(webhookURL, formatEnv)
	eventID := uuid.New().String()
	body, err := marshalWebhookBody(format, movieID, title, status, errMsg, eventID)
	if err != nil {
		logutils.Log.WithError(err).WithField("movie_id", movieID).Warn("Webhook: failed to marshal payload")
		return
	}
	go doSendWithRetry(webhookURL, authToken, body, movieID, eventID, format)
}

func doSendWithRetry(webhookURL, authToken string, body []byte, movieID uint, eventID, format string) {
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
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			fields := map[string]any{"movie_id": movieID, "event_id": eventID, "attempt": attempt + 1, "format": format}
			if strings.Contains(err.Error(), "connection refused") && strings.Contains(webhookURL, "127.0.0.1") {
				fields["hint"] = "from Docker, 127.0.0.1 is the container itself; use host.docker.internal (Desktop) or host-gateway (Linux compose)"
			}
			logutils.Log.WithError(err).WithFields(fields).Warn("Webhook: request failed")
			continue
		}
		sniff, _ := io.ReadAll(io.LimitReader(resp.Body, webhookBodySniff))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logutils.Log.WithFields(map[string]any{"movie_id": movieID, "event_id": eventID, "format": format}).Debug("Webhook delivered")
			return
		}
		fields := map[string]any{
			"movie_id": movieID, "event_id": eventID, "status": resp.StatusCode, "attempt": attempt + 1,
			"format": format, "body_snippet": strings.TrimSpace(string(sniff)),
		}
		if resp.StatusCode == http.StatusUnauthorized {
			fields["hint"] = "check TMS_WEBHOOK_TOKEN matches OpenClaw hooks.token (Bearer)"
		}
		logutils.Log.WithFields(fields).
			Warn("Webhook: non-2xx response (OpenClaw: URL path /hooks/wake vs /hooks/tms, hooks.mappings, token, payload format)")
	}
	logutils.Log.WithFields(map[string]any{
		"movie_id": movieID, "event_id": eventID, "attempts": webhookMaxRetries + 1, "format": format,
	}).Warn("Webhook: failed to deliver after all retries")
}
