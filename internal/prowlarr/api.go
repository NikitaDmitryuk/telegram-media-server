package prowlarr

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/go-resty/resty/v2"
)

type Prowlarr struct {
	Client  *resty.Client
	ApiKey  string
	BaseURL string
}

type TorrentSearchResult struct {
	Title       string
	Size        int64
	Magnet      string
	TorrentURL  string
	IndexerName string
	InfoHash    string
	Peers       int
}

type TorrentSearchPage struct {
	Results []TorrentSearchResult
	Total   int
	Offset  int
	Limit   int
}

func NewProwlarr(baseURL, apiKey string) *Prowlarr {
	client := resty.New().SetBaseURL(baseURL).SetHeader("X-Api-Key", apiKey)
	logutils.Log.Infof("Initialized Prowlarr client with baseURL: %s", baseURL)
	return &Prowlarr{
		Client:  client,
		ApiKey:  apiKey,
		BaseURL: baseURL,
	}
}

func (p *Prowlarr) SearchTorrents(query string, offset, limit int, indexerIDs, categories []int) (TorrentSearchPage, error) {
	logutils.Log.Infof(
		"Searching torrents: query='%s', offset=%d, limit=%d, indexers=%v, categories=%v",
		query, offset, limit, indexerIDs, categories,
	)
	page, err := p.searchTorrents(context.Background(), query, offset, limit, indexerIDs, categories)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to perform search request to Prowlarr")
		return TorrentSearchPage{}, err
	}
	logutils.Log.Infof("Prowlarr search returned %d results", len(page.Results))
	return page, nil
}

// SearchTorrentsWithContext is like SearchTorrents but uses ctx for timeout/cancellation (e.g. 10–15s for API).
func (p *Prowlarr) SearchTorrentsWithContext(
	ctx context.Context,
	query string,
	offset, limit int,
	indexerIDs, categories []int,
) (TorrentSearchPage, error) {
	return p.searchTorrents(ctx, query, offset, limit, indexerIDs, categories)
}

func (p *Prowlarr) searchTorrents(
	ctx context.Context,
	query string,
	offset, limit int,
	indexerIDs, categories []int,
) (TorrentSearchPage, error) {
	params := url.Values{}
	params.Set("query", query)
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if len(indexerIDs) > 0 {
		parts := make([]string, len(indexerIDs))
		for i, id := range indexerIDs {
			parts[i] = strconv.Itoa(id)
		}
		params.Set("indexerIds", strings.Join(parts, ","))
	}
	for _, cat := range categories {
		params.Add("categories", strconv.Itoa(cat))
	}
	params.Set("type", "search")

	req := p.Client.R().
		SetContext(ctx).
		SetQueryString(params.Encode()).
		SetHeader("X-Api-Key", p.ApiKey).
		SetResult(&[]map[string]any{})
	resp, err := req.Get("/api/v1/search")
	if err != nil {
		return TorrentSearchPage{}, fmt.Errorf("failed to perform search request: %w", err)
	}
	if resp.IsError() {
		return TorrentSearchPage{}, fmt.Errorf("prowlarr search error: %s", resp.Status())
	}
	var rawResults []map[string]any
	if result, ok := resp.Result().(*[]map[string]any); ok && result != nil {
		rawResults = *result
	} else {
		return TorrentSearchPage{}, fmt.Errorf("failed to parse search response")
	}
	results := make([]TorrentSearchResult, 0, len(rawResults))
	for _, r := range rawResults {
		title, _ := r["title"].(string)
		size, _ := r["size"].(float64)
		magnet, _ := r["magnetUrl"].(string)
		torrentURL, _ := r["downloadUrl"].(string)
		indexerName, _ := r["indexerName"].(string)
		infoHash, _ := r["infoHash"].(string)
		peers := 0
		if v, ok := r["peers"].(float64); ok {
			peers = int(v)
		} else {
			seeders, _ := r["seeders"].(float64)
			leechers, _ := r["leechers"].(float64)
			if seeders > 0 || leechers > 0 {
				peers = int(seeders + leechers)
			}
		}
		results = append(results, TorrentSearchResult{
			Title:       title,
			Size:        int64(size),
			Magnet:      magnet,
			TorrentURL:  torrentURL,
			IndexerName: indexerName,
			InfoHash:    infoHash,
			Peers:       peers,
		})
	}
	return TorrentSearchPage{
		Results: results,
		Total:   len(results),
		Offset:  offset,
		Limit:   limit,
	}, nil
}

type Indexer struct {
	ID   int
	Name string
}

func (p *Prowlarr) GetIndexers() ([]Indexer, error) {
	logutils.Log.Info("Requesting indexer list from Prowlarr")
	resp, err := p.Client.R().SetHeader("X-Api-Key", p.ApiKey).SetResult(&[]map[string]any{}).Get("/api/v1/indexer")
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to request indexers from Prowlarr")
		return nil, fmt.Errorf("failed to request indexers: %w", err)
	}
	if resp.IsError() {
		logutils.Log.WithField("status", resp.Status()).Warn("Prowlarr indexer request returned error status")
		return nil, fmt.Errorf("prowlarr indexer error: %s", resp.Status())
	}
	var raw []map[string]any
	if result, ok := resp.Result().(*[]map[string]any); ok && result != nil {
		raw = *result
	} else {
		logutils.Log.Error("Failed to parse indexer response from Prowlarr")
		return nil, fmt.Errorf("failed to parse indexer response")
	}
	res := make([]Indexer, 0, len(raw))
	for _, r := range raw {
		id, _ := r["id"].(float64)
		name, _ := r["name"].(string)
		res = append(res, Indexer{ID: int(id), Name: name})
	}
	logutils.Log.Infof("Prowlarr returned %d indexers", len(res))
	return res, nil
}

func (p *Prowlarr) GetTorrentFile(torrentURL string) ([]byte, error) {
	logutils.Log.Infof("Downloading torrent file from URL: %s", torrentURL)
	resp, err := p.Client.R().SetHeader("X-Api-Key", p.ApiKey).Get(torrentURL)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to download torrent file from Prowlarr")
		return nil, fmt.Errorf("failed to download torrent file: %w", err)
	}
	if resp.IsError() {
		logutils.Log.WithField("status", resp.Status()).Warn("Prowlarr torrent download returned error status")
		return nil, fmt.Errorf("prowlarr torrent download error: %s", resp.Status())
	}
	logutils.Log.Info("Torrent file downloaded successfully from Prowlarr")
	return resp.Body(), nil
}
