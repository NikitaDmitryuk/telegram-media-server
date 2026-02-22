package api

import (
	"context"
	"embed"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/app"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/google/uuid"
)

const jsonContentType = "application/json"

const (
	apiV1Prefix     = "/api/v1"
	healthPath      = apiV1Prefix + "/health"
	downloadsPath   = apiV1Prefix + "/downloads"
	searchPath      = apiV1Prefix + "/search"
	openapiYAMLPath = apiV1Prefix + "/openapi.yaml"
	openapiLLMPath  = apiV1Prefix + "/openapi-llm.yaml"
	swaggerDocsPath = apiV1Prefix + "/docs"
)

//go:embed openapi/openapi.yaml openapi/openapi-llm.yaml openapi/swagger-ui.html
var openapiFS embed.FS

// Handler is an API handler that receives the app.
type Handler func(http.ResponseWriter, *http.Request, *app.App)

// Server runs the TMS REST API.
type Server struct {
	app    *app.App
	apiKey string
	srv    *http.Server
}

// NewServer creates a new API server. When apiKey is empty, only requests from localhost are accepted.
func NewServer(a *app.App, listenAddr, apiKey string) *Server {
	s := &Server{app: a, apiKey: apiKey}
	mux := http.NewServeMux()

	// Documentation (when apiKey is empty, only localhost can access)
	mux.HandleFunc(openapiYAMLPath, s.docHandler(serveOpenAPIYAML))
	mux.HandleFunc(openapiLLMPath, s.docHandler(serveOpenAPILLMYAML))
	mux.HandleFunc(swaggerDocsPath, s.docHandler(serveSwaggerUI))

	mux.HandleFunc(healthPath, s.chain(s.healthHandler))
	mux.HandleFunc(downloadsPath, s.chain(s.downloadsHandler))
	mux.HandleFunc(downloadsPath+"/", s.chain(s.downloadByIDHandler))
	mux.HandleFunc(searchPath, s.chain(s.searchHandler))

	s.srv = &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

// isPrivateIP reports whether ip is in 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16.
func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return false
}

// isLocalhostOrAllowedInDocker returns true if the request is from localhost, or from a
// private IP when RUNNING_IN_DOCKER=true (host accessing via port mapping).
func isLocalhostOrAllowedInDocker(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	if host == "127.0.0.1" || host == "::1" {
		return true
	}
	if os.Getenv("RUNNING_IN_DOCKER") != "true" {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && isPrivateIP(ip)
}

// docHandler wraps a doc handler: when apiKey is empty, only localhost (or Docker host) is allowed.
func (s *Server) docHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" && !isLocalhostOrAllowedInDocker(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

// chain runs requestID then auth then the handler.
func (s *Server) chain(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		ctx = WithRequestID(ctx, requestID)
		w.Header().Set("X-Request-ID", requestID)
		r = r.WithContext(ctx)

		if s.apiKey != "" {
			token := ""
			if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
				token = strings.TrimSpace(ah[7:])
			}
			if token == "" {
				token = r.Header.Get("X-API-Key")
			}
			if token != s.apiKey {
				logutils.Log.WithFields(map[string]any{
					"request_id": requestID,
					"path":       r.URL.Path,
				}).Warn("API request unauthorized")
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		} else if !isLocalhostOrAllowedInDocker(r) {
			logutils.Log.WithFields(map[string]any{
				"request_id":  requestID,
				"path":        r.URL.Path,
				"remote_addr": r.RemoteAddr,
			}).Warn("API request rejected: non-localhost without API key")
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		logutils.Log.WithFields(map[string]any{
			"request_id": requestID,
			"path":       r.URL.Path,
			"method":     r.Method,
		}).Debug("API request")
		h(w, r, s.app)
	}
}

func (*Server) healthHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	Health(w, r, a)
}

func (*Server) downloadsHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	switch r.Method {
	case http.MethodGet:
		ListDownloads(w, r, a)
	case http.MethodPost:
		AddDownload(w, r, a)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (*Server) downloadByIDHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	base := path.Base(r.URL.Path)
	id, err := strconv.ParseUint(base, 10, 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid download id")
		return
	}
	DeleteDownload(w, r, a, uint(id))
}

func (*Server) searchHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	Search(w, r, a)
}

func serveOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, err := openapiFS.ReadFile("openapi/openapi.yaml")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "openapi spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func serveOpenAPILLMYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, err := openapiFS.ReadFile("openapi/openapi-llm.yaml")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "openapi-llm spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, err := openapiFS.ReadFile("openapi/swagger-ui.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "swagger ui not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// Start listens and serves. Blocks until Shutdown is called.
func (s *Server) Start() error {
	logutils.Log.WithField("addr", s.srv.Addr).Info("TMS API server starting")
	return s.srv.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logutils.Log.WithError(err).Warn("Failed to encode JSON response")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
