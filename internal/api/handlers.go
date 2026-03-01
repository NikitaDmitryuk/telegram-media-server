package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/factory"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/notifier"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

// Health returns 200 and {"status":"ok"}.
func Health(w http.ResponseWriter, _ *http.Request, _ *app.App) {
	resp := HealthResponse{Status: "ok"}
	writeJSON(w, http.StatusOK, resp)
}

// ListDownloads returns GET /api/v1/downloads — best effort snapshot of queue + active + DB.
func ListDownloads(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	var items []DownloadItem

	// Queue items
	for _, q := range a.DownloadManager.GetQueueItems() {
		movieID, _ := uintFromMap(q, "movie_id")
		title, _ := q["title"].(string)
		pos, _ := q["position"].(int)
		items = append(items, DownloadItem{
			ID:              movieID,
			Title:           title,
			Status:          "queued",
			Progress:        0,
			PositionInQueue: &pos,
		})
	}

	// Active downloads (from jobs)
	for _, movieID := range a.DownloadManager.GetActiveDownloads() {
		movie, err := a.DB.GetMovieByID(ctx, movieID)
		if err != nil {
			logutils.Log.WithError(err).WithFields(map[string]any{
				"movie_id":   movieID,
				"request_id": RequestIDFromContext(ctx),
			}).Warn("ListDownloads: skip active download (GetMovieByID failed)")
			continue
		}
		status := downloadStatusFromMovie(&movie)
		items = append(items, DownloadItem{
			ID:                 movie.ID,
			Title:              movie.Name,
			Status:             status,
			Progress:           movie.DownloadedPercentage,
			ConversionProgress: movie.ConversionPercentage,
		})
	}

	writeJSON(w, http.StatusOK, items)
}

const downloadPercentComplete = 100

func downloadStatusFromMovie(m *database.Movie) string {
	if m.DownloadedPercentage < downloadPercentComplete {
		return "downloading"
	}
	switch m.ConversionStatus {
	case "pending", "in_progress":
		return "converting"
	case "done", "skipped", "":
		return "completed"
	case "failed":
		return "failed"
	default:
		return "completed"
	}
}

func uintFromMap(m map[string]any, key string) (uint, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case float64:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case uint:
		return n, true
	}
	return 0, false
}

// DeleteDownload handles DELETE /api/v1/downloads/:id.
func DeleteDownload(w http.ResponseWriter, r *http.Request, a *app.App, id uint) {
	err := a.DownloadManager.StopDownload(id)
	if err != nil {
		logutils.Log.WithError(err).WithFields(map[string]any{
			"movie_id":   id,
			"request_id": RequestIDFromContext(r.Context()),
		}).Error("DeleteDownload: StopDownload failed")
		writeError(w, http.StatusInternalServerError, "failed to stop download")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxAddDownloadBodyBytes limits POST /api/v1/downloads body size to avoid DoS.
const maxAddDownloadBodyBytes = 1024 * 1024 // 1 MiB

// AddDownload handles POST /api/v1/downloads. Body: {"url":"..."}.
func AddDownload(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	body := http.MaxBytesReader(w, r.Body, maxAddDownloadBodyBytes)
	var req AddDownloadRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	dl, err := factory.CreateDownloaderFromURL(ctx, req.URL, a.Config.MoviePath, a.Config)
	if err != nil {
		logutils.Log.WithError(err).WithField("request_id", RequestIDFromContext(ctx)).Debug("AddDownload: CreateDownloaderFromURL failed")
		writeError(w, http.StatusBadRequest, utils.DownloadErrorMessage(err))
		return
	}
	title, _ := dl.GetTitle()
	if validateErr := app.ValidateDownloadStart(ctx, a, dl); validateErr != nil {
		logutils.Log.WithError(validateErr).
			WithField("request_id", RequestIDFromContext(ctx)).
			Debug("AddDownload: ValidateDownloadStart failed")
		switch {
		case errors.Is(validateErr, app.ErrAlreadyExists):
			writeError(w, http.StatusConflict, "media already exists")
			return
		case errors.Is(validateErr, app.ErrNotEnoughSpace):
			writeError(w, http.StatusInsufficientStorage, "not enough disk space")
			return
		default:
			writeError(w, http.StatusBadRequest, utils.DownloadErrorMessage(validateErr))
			return
		}
	}
	movieID, _, completionChan, err := a.DownloadManager.StartDownload(dl, notifier.Noop)
	if err != nil {
		logutils.Log.WithError(err).WithField("request_id", RequestIDFromContext(ctx)).Error("AddDownload: StartDownload failed")
		writeError(w, http.StatusInternalServerError, utils.DownloadErrorMessage(err))
		return
	}
	if strings.TrimSpace(req.Title) != "" {
		if updateErr := a.DB.UpdateMovieName(ctx, movieID, strings.TrimSpace(req.Title)); updateErr != nil {
			logutils.Log.WithError(updateErr).
				WithField("movie_id", movieID).
				Warn("AddDownload: UpdateMovieName failed, using downloader title")
		} else {
			title = strings.TrimSpace(req.Title)
		}
	}
	go app.RunCompletionLoop(a, completionChan, dl, movieID, title, webhookNotifier{
		webhookURL:   a.Config.TMSWebhookURL,
		webhookToken: a.Config.TMSWebhookToken,
	})
	writeJSON(w, http.StatusCreated, AddDownloadResponse{ID: movieID, Title: title})
}

// webhookNotifier implements notifier.CompletionNotifier for API-originated downloads.
type webhookNotifier struct {
	webhookURL   string
	webhookToken string
}

func (n webhookNotifier) OnStopped(movieID uint, title string) {
	SendCompletionWebhook(n.webhookURL, n.webhookToken, movieID, title, "stopped", "")
}

func (n webhookNotifier) OnFailed(movieID uint, title string, err error) {
	SendCompletionWebhook(n.webhookURL, n.webhookToken, movieID, title, "failed", utils.DownloadErrorMessage(err))
}

func (n webhookNotifier) OnCompleted(movieID uint, title string) {
	SendCompletionWebhook(n.webhookURL, n.webhookToken, movieID, title, "completed", "")
}

const prowlarrSearchTimeout = 15 * time.Second

// Search handles GET /api/v1/search?q=...&limit=...&quality=...
func Search(w http.ResponseWriter, r *http.Request, a *app.App) {
	if a.Config.ProwlarrURL == "" || a.Config.ProwlarrAPIKey == "" {
		writeError(w, http.StatusServiceUnavailable, "search not configured (Prowlarr)")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), prowlarrSearchTimeout)
	defer cancel()
	client := prowlarr.NewProwlarr(a.Config.ProwlarrURL, a.Config.ProwlarrAPIKey)
	page, err := client.SearchTorrentsWithContext(ctx, q, 0, limit, nil, nil)
	if err != nil {
		logutils.Log.WithError(err).WithField("request_id", RequestIDFromContext(r.Context())).Error("Search: Prowlarr request failed")
		writeError(w, http.StatusServiceUnavailable, "search unavailable")
		return
	}
	quality := strings.TrimSpace(r.URL.Query().Get("quality"))
	items := make([]SearchResultItem, 0, len(page.Results))
	for _, res := range page.Results {
		if quality != "" && !strings.Contains(strings.ToLower(res.Title), strings.ToLower(quality)) {
			continue
		}
		items = append(items, SearchResultItem{
			Title:       res.Title,
			Size:        res.Size,
			Magnet:      res.Magnet,
			TorrentURL:  res.TorrentURL,
			IndexerName: res.IndexerName,
			Peers:       res.Peers,
		})
	}
	writeJSON(w, http.StatusOK, items)
}
