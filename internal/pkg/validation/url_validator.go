package validation

import (
	"context"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/errors"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

// DefaultURLValidator реализует валидацию URL для видео и торрентов
type DefaultURLValidator struct{}

// NewDefaultURLValidator создает новый валидатор URL
func NewDefaultURLValidator() domain.URLValidator {
	return &DefaultURLValidator{}
}

// IsValidVideoURL проверяет, является ли URL валидным для видео
func (v *DefaultURLValidator) IsValidVideoURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	// Проверяем основной формат URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Проверяем схему
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Проверяем популярные видео домены
	host := strings.ToLower(parsedURL.Host)
	videoHosts := []string{
		"youtube.com", "www.youtube.com", "youtu.be", "m.youtube.com",
		"vk.com", "www.vk.com",
		"rutube.ru", "www.rutube.ru",
		"vimeo.com", "www.vimeo.com",
		"dailymotion.com", "www.dailymotion.com",
		"twitch.tv", "www.twitch.tv",
		"ok.ru", "www.ok.ru",
	}

	for _, validHost := range videoHosts {
		if host == validHost || strings.HasSuffix(host, "."+validHost) {
			return true
		}
	}

	// Проверяем через yt-dlp, если доступен
	return v.checkWithYTDLP(rawURL)
}

// IsValidTorrentURL проверяет, является ли URL валидным для торрентов
func (v *DefaultURLValidator) IsValidTorrentURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	// Проверяем magnet ссылки
	if strings.HasPrefix(rawURL, "magnet:") {
		return v.isValidMagnetLink(rawURL)
	}

	// Проверяем HTTP/HTTPS ссылки на .torrent файлы
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Проверяем расширение файла
	return strings.HasSuffix(strings.ToLower(parsedURL.Path), ".torrent")
}

// ExtractVideoInfo извлекает информацию о видео (базовая реализация)
func (v *DefaultURLValidator) ExtractVideoInfo(rawURL string) (*domain.VideoInfo, error) {
	if !v.IsValidVideoURL(rawURL) {
		return nil, errors.NewDomainError(
			errors.ErrorTypeValidation,
			"invalid_video_url",
			"provided URL is not a valid video URL",
		)
	}

	// Базовая информация
	return &domain.VideoInfo{
		Title:       extractTitleFromURL(rawURL),
		Duration:    0, // Будет получена при загрузке
		Quality:     "unknown",
		Size:        0,
		Thumbnail:   "",
		Description: "",
	}, nil
}

// checkWithYTDLP проверяет URL через yt-dlp
func (v *DefaultURLValidator) checkWithYTDLP(rawURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", "--simulate", "--quiet", rawURL)
	err := cmd.Run()
	if err != nil {
		logger.Log.WithError(err).Debug("yt-dlp validation failed for URL: " + rawURL)
		return false
	}

	return true
}

// isValidMagnetLink проверяет валидность magnet ссылки
func (v *DefaultURLValidator) isValidMagnetLink(magnetURI string) bool {
	// Базовая регулярка для magnet ссылок
	magnetRegex := regexp.MustCompile(`^magnet:\?xt=urn:btih:[a-fA-F0-9]{40}&.*`)
	return magnetRegex.MatchString(magnetURI)
}

// extractTitleFromURL извлекает заголовок из URL (базовая реализация)
func extractTitleFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "Unknown Video"
	}

	// Для YouTube пытаемся извлечь заголовок из URL
	if strings.Contains(parsedURL.Host, "youtube.com") || strings.Contains(parsedURL.Host, "youtu.be") {
		return "YouTube Video"
	}

	if strings.Contains(parsedURL.Host, "vk.com") {
		return "VK Video"
	}

	if strings.Contains(parsedURL.Host, "rutube.ru") {
		return "RuTube Video"
	}

	return "Video from " + parsedURL.Host
}

// MockURLValidator для тестирования
type MockURLValidator struct {
	ValidVideoURLs   []string
	ValidTorrentURLs []string
	VideoInfo        *domain.VideoInfo
	ShouldError      bool
}

// NewMockURLValidator создает mock валидатор для тестов
func NewMockURLValidator() *MockURLValidator {
	return &MockURLValidator{
		ValidVideoURLs:   make([]string, 0),
		ValidTorrentURLs: make([]string, 0),
		VideoInfo: &domain.VideoInfo{
			Title:       "Test Video",
			Duration:    time.Minute * 5,
			Quality:     "720p",
			Size:        1024 * 1024 * 100, // 100MB
			Thumbnail:   "https://example.com/thumb.jpg",
			Description: "Test video description",
		},
	}
}

// IsValidVideoURL для мока
func (m *MockURLValidator) IsValidVideoURL(rawURL string) bool {
	for _, valid := range m.ValidVideoURLs {
		if rawURL == valid {
			return true
		}
	}
	return false
}

// IsValidTorrentURL для мока
func (m *MockURLValidator) IsValidTorrentURL(rawURL string) bool {
	for _, valid := range m.ValidTorrentURLs {
		if rawURL == valid {
			return true
		}
	}
	return false
}

// ExtractVideoInfo для мока
func (m *MockURLValidator) ExtractVideoInfo(rawURL string) (*domain.VideoInfo, error) {
	if m.ShouldError {
		return nil, errors.NewDomainError(
			errors.ErrorTypeValidation,
			"mock_error",
			"mock validation error",
		)
	}

	if !m.IsValidVideoURL(rawURL) {
		return nil, errors.NewDomainError(
			errors.ErrorTypeValidation,
			"invalid_video_url",
			"mock: URL is not valid",
		)
	}

	return m.VideoInfo, nil
}

// AddValidVideoURL добавляет валидный URL для видео
func (m *MockURLValidator) AddValidVideoURL(url string) {
	m.ValidVideoURLs = append(m.ValidVideoURLs, url)
}

// AddValidTorrentURL добавляет валидный URL для торрента
func (m *MockURLValidator) AddValidTorrentURL(url string) {
	m.ValidTorrentURLs = append(m.ValidTorrentURLs, url)
}

// SetVideoInfo устанавливает информацию о видео для мока
func (m *MockURLValidator) SetVideoInfo(info *domain.VideoInfo) {
	m.VideoInfo = info
}

// SetShouldError устанавливает, должен ли мок возвращать ошибку
func (m *MockURLValidator) SetShouldError(shouldError bool) {
	m.ShouldError = shouldError
}
