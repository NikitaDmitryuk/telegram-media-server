package prowlarr

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
)

// Prowlarr реализует интерфейс ProwlarrInterface
type Prowlarr struct {
	httpClient domain.HTTPClientInterface
	apiKey     string
	baseURL    string
}

// Проверяем, что Prowlarr реализует интерфейс ProwlarrInterface
var _ domain.ProwlarrInterface = (*Prowlarr)(nil)

// NewProwlarr создает новый клиент Prowlarr
func NewProwlarr(httpClient domain.HTTPClientInterface, baseURL, apiKey string) domain.ProwlarrInterface {
	client := httpClient.SetBaseURL(baseURL).SetHeader("X-Api-Key", apiKey)
	logger.Log.Infof("Initialized Prowlarr client with baseURL: %s", baseURL)
	return &Prowlarr{
		httpClient: client,
		apiKey:     apiKey,
		baseURL:    baseURL,
	}
}

func (p *Prowlarr) SearchTorrents(query string, offset, limit int, indexerIDs, categories []int) (domain.TorrentSearchPage, error) {
	params := url.Values{}
	params.Set("query", query)
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if len(indexerIDs) > 0 {
		ids := ""
		for i, id := range indexerIDs {
			if i > 0 {
				ids += ","
			}
			ids += strconv.Itoa(id)
		}
		params.Set("indexerIds", ids)
	}
	for _, cat := range categories {
		params.Add("categories", strconv.Itoa(cat))
	}
	params.Set("type", "search")

	logger.Log.Infof(
		"Searching torrents: query='%s', offset=%d, limit=%d, indexers=%v, categories=%v",
		query, offset, limit, indexerIDs, categories,
	)

	searchURL := "/api/v1/search?" + params.Encode()
	resp, err := p.httpClient.Get(searchURL)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to perform search request to Prowlarr")
		return domain.TorrentSearchPage{}, utils.WrapError(err, "failed to perform search request", map[string]any{
			"query": query,
		})
	}

	if resp.IsError {
		logger.Log.WithField("status", resp.StatusCode).Warn("Prowlarr search returned error status")
		return domain.TorrentSearchPage{}, fmt.Errorf("prowlarr search error: status %d", resp.StatusCode)
	}

	var rawResults []map[string]any
	err = json.Unmarshal(resp.Body, &rawResults)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse search response from Prowlarr")
		return domain.TorrentSearchPage{}, utils.WrapError(err, "failed to parse search response", nil)
	}

	results := make([]domain.TorrentSearchResult, 0, len(rawResults))
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
		results = append(results, domain.TorrentSearchResult{
			Title:       title,
			Size:        int64(size),
			Magnet:      magnet,
			TorrentURL:  torrentURL,
			IndexerName: indexerName,
			InfoHash:    infoHash,
			Peers:       peers,
		})
	}
	logger.Log.Infof("Prowlarr search returned %d results", len(results))
	return domain.TorrentSearchPage{
		Results: results,
		Total:   len(results),
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func (p *Prowlarr) GetIndexers() ([]domain.Indexer, error) {
	logger.Log.Info("Requesting indexer list from Prowlarr")

	resp, err := p.httpClient.Get("/api/v1/indexer")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to request indexers from Prowlarr")
		return nil, utils.WrapError(err, "failed to request indexers", nil)
	}

	if resp.IsError {
		logger.Log.WithField("status", resp.StatusCode).Warn("Prowlarr indexer request returned error status")
		return nil, fmt.Errorf("prowlarr indexer error: status %d", resp.StatusCode)
	}

	var raw []map[string]any
	err = json.Unmarshal(resp.Body, &raw)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse indexer response from Prowlarr")
		return nil, utils.WrapError(err, "failed to parse indexer response", nil)
	}

	res := make([]domain.Indexer, 0, len(raw))
	for _, r := range raw {
		id, _ := r["id"].(float64)
		name, _ := r["name"].(string)
		res = append(res, domain.Indexer{ID: int(id), Name: name})
	}
	logger.Log.Infof("Prowlarr returned %d indexers", len(res))
	return res, nil
}

func (p *Prowlarr) GetTorrentFile(torrentURL string) ([]byte, error) {
	logger.Log.Infof("Downloading torrent file from URL: %s", torrentURL)

	resp, err := p.httpClient.Get(torrentURL)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to download torrent file from Prowlarr")
		return nil, utils.WrapError(err, "failed to download torrent file", map[string]any{
			"url": torrentURL,
		})
	}

	if resp.IsError {
		logger.Log.WithField("status", resp.StatusCode).Warn("Prowlarr torrent download returned error status")
		return nil, fmt.Errorf("prowlarr torrent download error: status %d", resp.StatusCode)
	}

	logger.Log.Info("Torrent file downloaded successfully from Prowlarr")
	return resp.Body, nil
}
