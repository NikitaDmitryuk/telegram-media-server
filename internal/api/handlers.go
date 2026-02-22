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
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/factory"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/prowlarr"
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
		writeError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	title, _ := dl.GetTitle()
	movieID, progressChan, errChan, err := a.DownloadManager.StartDownload(dl, 0)
	if err != nil {
		logutils.Log.WithError(err).WithField("request_id", RequestIDFromContext(ctx)).Error("AddDownload: StartDownload failed")
		writeError(w, http.StatusInternalServerError, "failed to start download")
		return
	}
	go handleAPIDownloadCompletion(a, dl, movieID, title, progressChan, errChan)
	writeJSON(w, http.StatusCreated, AddDownloadResponse{ID: movieID, Title: title})
}

// handleAPIDownloadCompletion drains progress/error channels and runs webhook + temp file cleanup for API-originated downloads.
// Mirrors handleDownloadCompletion in internal/handlers/downloads/common.go for chatID=0.
func handleAPIDownloadCompletion(
	a *app.App,
	dl downloader.Downloader,
	movieID uint,
	title string,
	progressChan <-chan float64,
	errChan <-chan error,
) {
	for range progressChan {
	}
	err := <-errChan
	if errors.Is(err, downloader.ErrStoppedByDeletion) {
		logutils.Log.Info("API download stopped by deletion queue (no notification)")
		return
	}
	if dl.StoppedManually() {
		logutils.Log.Info("API download was manually stopped")
		if a.Config.TMSWebhookURL != "" {
			SendCompletionWebhook(a.Config.TMSWebhookURL, movieID, title, "stopped", "")
		}
		return
	}
	if err != nil {
		logutils.Log.WithError(err).Error("API download failed")
		if deleteErr := filemanager.DeleteMovie(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); deleteErr != nil {
			logutils.Log.WithError(deleteErr).Error("Failed to delete movie after API download failed")
		}
		if a.Config.TMSWebhookURL != "" {
			SendCompletionWebhook(a.Config.TMSWebhookURL, movieID, title, "failed", err.Error())
		}
		return
	}
	logutils.Log.Info("API download completed successfully")
	if err := filemanager.DeleteTemporaryFilesByMovieID(movieID, a.Config.MoviePath, a.DB, a.DownloadManager); err != nil {
		logutils.Log.WithError(err).Error("Failed to delete temporary files after API download")
	}
	if a.Config.TMSWebhookURL != "" {
		SendCompletionWebhook(a.Config.TMSWebhookURL, movieID, title, "completed", "")
	}
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
