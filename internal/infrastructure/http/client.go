package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// Client реализует интерфейс HTTPClientInterface
type Client struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// Проверяем, что Client реализует интерфейс HTTPClientInterface
var _ domain.HTTPClientInterface = (*Client)(nil)

// NewHTTPClient создает новый HTTP клиент
func NewHTTPClient() domain.HTTPClientInterface {
	return &Client{
		client:  &http.Client{},
		headers: make(map[string]string),
	}
}

// SetBaseURL устанавливает базовый URL
func (c *Client) SetBaseURL(url string) domain.HTTPClientInterface {
	c.baseURL = url
	return c
}

// SetHeader устанавливает заголовок
func (c *Client) SetHeader(key, value string) domain.HTTPClientInterface {
	c.headers[key] = value
	return c
}

// Get выполняет GET запрос
func (c *Client) Get(url string) (*domain.HTTPResponse, error) {
	ctx := context.Background()
	fullURL := c.buildURL(url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	// Добавляем заголовки
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	logger.Log.WithField("url", fullURL).Debug("Making GET request")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GET request: %w", err)
	}
	defer resp.Body.Close()

	return c.buildResponse(resp)
}

// Post выполняет POST запрос
func (c *Client) Post(url string, body []byte) (*domain.HTTPResponse, error) {
	ctx := context.Background()
	fullURL := c.buildURL(url)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}

	// Добавляем заголовки
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	// Устанавливаем Content-Type по умолчанию
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	logger.Log.WithFields(map[string]any{
		"url":         fullURL,
		"body_length": len(body),
	}).Debug("Making POST request")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute POST request: %w", err)
	}
	defer resp.Body.Close()

	return c.buildResponse(resp)
}

// buildURL строит полный URL
func (c *Client) buildURL(url string) string {
	if c.baseURL == "" {
		return url
	}

	// Если URL уже полный, возвращаем как есть
	if len(url) > 7 && (url[:7] == "http://" || url[:8] == "https://") {
		return url
	}

	// Убираем слеш в начале относительного URL если есть
	if url != "" && url[0] == '/' {
		url = url[1:]
	}

	// Убираем слеш в конце базового URL если есть
	baseURL := c.baseURL
	if baseURL != "" && baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}

	return baseURL + "/" + url
}

// buildResponse создает объект ответа
func (*Client) buildResponse(resp *http.Response) (*domain.HTTPResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return &domain.HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    headers,
		IsError:    resp.StatusCode >= 400,
	}, nil
}
