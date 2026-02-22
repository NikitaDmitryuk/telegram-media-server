package api

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status string `json:"status"`
}

// DownloadItem is one entry in GET /api/v1/downloads (best effort snapshot).
type DownloadItem struct {
	ID                 uint   `json:"id"`
	Title              string `json:"title"`
	Status             string `json:"status"` // queued, downloading, converting, completed, failed, stopped
	Progress           int    `json:"progress"`
	ConversionProgress int    `json:"conversion_progress,omitempty"`
	Error              string `json:"error,omitempty"`
	PositionInQueue    *int   `json:"position_in_queue,omitempty"`
}

// AddDownloadRequest is the body for POST /api/v1/downloads.
type AddDownloadRequest struct {
	URL string `json:"url"`
}

// AddDownloadResponse is returned on success by POST /api/v1/downloads.
type AddDownloadResponse struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
}

// SearchResultItem is one entry in GET /api/v1/search (Prowlarr).
type SearchResultItem struct {
	Title       string `json:"title"`
	Size        int64  `json:"size"`
	Magnet      string `json:"magnet,omitempty"`
	TorrentURL  string `json:"torrent_url,omitempty"`
	IndexerName string `json:"indexer_name,omitempty"`
	Peers       int    `json:"peers"`
}
