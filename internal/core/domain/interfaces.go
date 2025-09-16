package domain

import (
	"context"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotInterface определяет интерфейс для работы с Telegram Bot API
type BotInterface interface {
	SendMessage(chatID int64, text string, keyboard any)
	SendMessageWithMarkup(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) error
	DownloadFile(fileID, fileName string) error
	SaveFile(fileName string, data []byte) error
	AnswerCallbackQuery(callbackConfig tgbotapi.CallbackConfig)
	DeleteMessage(chatID int64, messageID int) error
	GetFileDirectURL(fileID string) (string, error)
	GetConfig() *Config
	GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
}

// DownloadManagerInterface определяет интерфейс для управления загрузками
type DownloadManagerInterface interface {
	StartDownload(dl Downloader, chatID int64) (movieID uint, progressChan chan float64, errChan chan error, err error)
	StopDownload(movieID uint) error
	StopAllDownloads()
	RemoveFromQueue(movieID uint) bool
	GetQueueStatus() []QueueItem
}

// QueueItem представляет элемент очереди загрузок
type QueueItem struct {
	MovieID   uint
	Title     string
	ChatID    int64
	Position  int
	StartTime time.Time
}

// HTTPClientInterface определяет интерфейс для HTTP клиента
type HTTPClientInterface interface {
	Get(url string) (*HTTPResponse, error)
	Post(url string, body []byte) (*HTTPResponse, error)
	SetHeader(key, value string) HTTPClientInterface
	SetBaseURL(url string) HTTPClientInterface
}

// HTTPResponse представляет HTTP ответ
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
	IsError    bool
}

// ProwlarrInterface определяет интерфейс для работы с Prowlarr API
type ProwlarrInterface interface {
	SearchTorrents(query string, offset, limit int, indexerIDs, categories []int) (TorrentSearchPage, error)
	GetIndexers() ([]Indexer, error)
	GetTorrentFile(torrentURL string) ([]byte, error)
}

// TorrentSearchPage представляет страницу результатов поиска торрентов
type TorrentSearchPage struct {
	Results []TorrentSearchResult
	Total   int
	Offset  int
	Limit   int
}

// TorrentSearchResult представляет результат поиска торрента
type TorrentSearchResult struct {
	Title       string
	Size        int64
	Magnet      string
	TorrentURL  string
	IndexerName string
	InfoHash    string
	Peers       int
}

// Indexer представляет торрент-индексатор
type Indexer struct {
	ID   int
	Name string
}

// CommandInterface определяет интерфейс для команд
type CommandInterface interface {
	Execute(ctx context.Context) (*CommandResult, error)
	GetType() string
	Validate() error
}

// CommandResult представляет результат выполнения команды
type CommandResult struct {
	Message    string
	Keyboard   any
	ChatID     int64
	DeletePrev bool
	Error      error
}

// CommandHandlerInterface определяет интерфейс для обработчиков команд
type CommandHandlerInterface interface {
	Handle(ctx context.Context, cmd CommandInterface) (*CommandResult, error)
	CanHandle(cmd CommandInterface) bool
	GetPriority() int
}

// ServiceContainerInterface определяет интерфейс для DI контейнера
type ServiceContainerInterface interface {
	GetBot() BotInterface
	GetDatabase() Database
	GetDownloadManager() DownloadManagerInterface
	GetProwlarr() ProwlarrInterface
	GetConfig() *Config
	GetAuthService() AuthServiceInterface
	GetDownloadService() DownloadServiceInterface
	GetMovieService() MovieServiceInterface
}

// AuthServiceInterface определяет интерфейс для сервиса авторизации
type AuthServiceInterface interface {
	Login(ctx context.Context, password string, chatID int64, userName string) (*LoginResult, error)
	CheckAccess(ctx context.Context, chatID int64) (bool, UserRole, error)
	GenerateTempPassword(ctx context.Context, duration time.Duration) (string, error)
	IsAdmin(ctx context.Context, chatID int64) (bool, error)
}

// LoginResult представляет результат авторизации
type LoginResult struct {
	Success bool
	Role    UserRole
	Message string
}

// DownloadServiceInterface определяет интерфейс для сервиса загрузок
type DownloadServiceInterface interface {
	HandleVideoLink(ctx context.Context, link string, chatID int64) error
	HandleTorrentFile(ctx context.Context, fileData []byte, fileName string, chatID int64) error
	GetDownloadStatus(ctx context.Context, movieID uint) (*DownloadStatus, error)
	CancelDownload(ctx context.Context, movieID uint) error
}

// DownloadStatus представляет статус загрузки
type DownloadStatus struct {
	MovieID   uint
	Title     string
	Progress  float64
	Status    string
	Error     string
	StartTime time.Time
	FileSize  int64
}

// MovieServiceInterface определяет интерфейс для сервиса фильмов
type MovieServiceInterface interface {
	GetMovieList(ctx context.Context) ([]Movie, error)
	DeleteMovie(ctx context.Context, movieID uint) error
	DeleteAllMovies(ctx context.Context) error
	GetMovieByID(ctx context.Context, movieID uint) (*Movie, error)
}

// RateLimiterInterface определяет интерфейс для ограничения частоты запросов
type RateLimiterInterface interface {
	Allow(userID int64) bool
	Reset(userID int64)
	GetLimit(userID int64) int
	GetRemaining(userID int64) int
}

// MetricsInterface определяет интерфейс для сбора метрик
type MetricsInterface interface {
	IncrementCounter(name string, labels map[string]string)
	RecordDuration(name string, duration time.Duration, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
	RecordHistogram(name string, value float64, labels map[string]string)
}

// HealthCheckInterface определяет интерфейс для проверки здоровья сервисов
type HealthCheckInterface interface {
	Check(ctx context.Context) error
	Name() string
}

// GracefulShutdownInterface определяет интерфейс для graceful shutdown
type GracefulShutdownInterface interface {
	Shutdown(ctx context.Context) error
	Name() string
}

// DownloaderFactory создает различные типы загрузчиков
type DownloaderFactory interface {
	CreateVideoDownloader(ctx context.Context, url string, config *Config) (Downloader, error)
	CreateTorrentDownloader(ctx context.Context, fileName, moviePath string, config *Config) (Downloader, error)
}

// NotificationService отправляет уведомления пользователям
type NotificationService interface {
	NotifyDownloadStarted(ctx context.Context, chatID int64, title string) error
	NotifyDownloadProgress(ctx context.Context, chatID int64, title string, progress int) error
	NotifyDownloadCompleted(ctx context.Context, chatID int64, title string) error
	NotifyDownloadFailed(ctx context.Context, chatID int64, title string, err error) error
}

// FileSystemInterface абстрагирует операции с файловой системой
type FileSystemInterface interface {
	Exists(path string) bool
	CreateDir(path string) error
	RemoveFile(path string) error
	RemoveDir(path string) error
	WriteFile(path string, data []byte) error
	ReadFile(path string) ([]byte, error)
	ListFiles(dir string) ([]string, error)
	GetFileSize(path string) (int64, error)
	GetFileModTime(path string) (time.Time, error)
}

// ProcessExecutor выполняет внешние процессы
type ProcessExecutor interface {
	Execute(ctx context.Context, command string, args ...string) ([]byte, error)
	ExecuteWithProgress(ctx context.Context, command string, args []string, progressChan chan<- int) error
	Kill(pid int) error
}

// TimeProvider предоставляет функции работы со временем
type TimeProvider interface {
	Now() time.Time
	Sleep(duration time.Duration)
	After(duration time.Duration) <-chan time.Time
	NewTimer(duration time.Duration) Timer
	NewTicker(duration time.Duration) Ticker
}

// Timer интерфейс для таймера
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(duration time.Duration) bool
}

// Ticker интерфейс для тикера
type Ticker interface {
	C() <-chan time.Time
	Stop()
	Reset(duration time.Duration)
}

// URLValidator валидирует URL
type URLValidator interface {
	IsValidVideoURL(url string) bool
	IsValidTorrentURL(url string) bool
	ExtractVideoInfo(url string) (*VideoInfo, error)
}

// VideoInfo содержит информацию о видео
type VideoInfo struct {
	Title       string        `json:"title"`
	Duration    time.Duration `json:"duration"`
	Quality     string        `json:"quality"`
	Size        int64         `json:"size"`
	Thumbnail   string        `json:"thumbnail"`
	Description string        `json:"description"`
}

// ConfigValidator валидирует конфигурацию
type ConfigValidator interface {
	ValidateConfig(config *Config) error
	ValidateDownloadSettings(settings *DownloadConfig) error
	ValidateSecuritySettings(settings *SecurityConfig) error
}

// CacheInterface для кеширования данных
type CacheInterface interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
	Size() int
}

// EventBus для событийно-ориентированной архитектуры
type EventBus interface {
	Publish(event Event) error
	Subscribe(eventType string, handler EventHandler) error
	Unsubscribe(eventType string, handler EventHandler) error
}

// Event представляет событие в системе
type Event interface {
	Type() string
	Payload() any
	Timestamp() time.Time
	Source() string
}

// EventHandler обрабатывает события
type EventHandler interface {
	Handle(event Event) error
	CanHandle(eventType string) bool
}

// DownloadEvent события загрузки
type DownloadEvent struct {
	EventType   string    `json:"event_type"`
	DownloadID  string    `json:"download_id"`
	ChatID      int64     `json:"chat_id"`
	Title       string    `json:"title"`
	Progress    int       `json:"progress,omitempty"`
	Error       string    `json:"error,omitempty"`
	EventTime   time.Time `json:"event_time"`
	EventSource string    `json:"event_source"`
	Metadata    any       `json:"metadata,omitempty"`
}

// Type возвращает тип события
func (de *DownloadEvent) Type() string {
	return de.EventType
}

// Payload возвращает данные события
func (de *DownloadEvent) Payload() any {
	return de
}

// Timestamp возвращает время события
func (de *DownloadEvent) Timestamp() time.Time {
	return de.EventTime
}

// Source возвращает источник события
func (de *DownloadEvent) Source() string {
	return de.EventSource
}

// Константы для типов событий
const (
	EventDownloadStarted   = "download.started"
	EventDownloadProgress  = "download.progress"
	EventDownloadCompleted = "download.completed"
	EventDownloadFailed    = "download.failed"
	EventDownloadStopped   = "download.stopped"
)

// Database интерфейс для работы с базой данных
type Database interface {
	Init(config *Config) error
	Close() error
	AddMovie(ctx context.Context, name string, fileSize int64, mainFiles, tempFiles []string) (uint, error)
	RemoveMovie(ctx context.Context, movieID uint) error
	GetMovieList(ctx context.Context) ([]Movie, error)
	GetTempFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error)
	UpdateDownloadedPercentage(ctx context.Context, movieID uint, percentage int) error
	SetLoaded(ctx context.Context, movieID uint) error
	GetMovieByID(ctx context.Context, movieID uint) (Movie, error)
	MovieExistsFiles(ctx context.Context, files []string) (bool, error)
	MovieExistsId(ctx context.Context, movieID uint) (bool, error)
	GetFilesByMovieID(ctx context.Context, movieID uint) ([]MovieFile, error)
	RemoveFilesByMovieID(ctx context.Context, movieID uint) error
	RemoveTempFilesByMovieID(ctx context.Context, movieID uint) error
	MovieExistsUploadedFile(ctx context.Context, fileName string) (bool, error)
	Login(ctx context.Context, password string, chatID int64, userName string, config *Config) (bool, error)
	GetUserRole(ctx context.Context, chatID int64) (UserRole, error)
	IsUserAccessAllowed(ctx context.Context, chatID int64) (bool, UserRole, error)
	AssignTemporaryPassword(ctx context.Context, password string, chatID int64) error
	ExtendTemporaryUser(ctx context.Context, chatID int64, newExpiration time.Time) error
	GenerateTemporaryPassword(ctx context.Context, duration time.Duration) (string, error)
	GetUserByChatID(ctx context.Context, chatID int64) (User, error)
}

// Downloader defines the interface for all downloaders
type Downloader interface {
	GetTitle() (string, error)
	GetFiles() ([]string, []string, error)
	GetFileSize() (int64, error)
	StoppedManually() bool
	StartDownload(ctx context.Context) (chan float64, chan error, error)
	StopDownload() error
}

// Config представляет конфигурацию приложения
type Config struct {
	BotToken        string
	MoviePath       string
	AdminPassword   string
	RegularPassword string
	Lang            string
	Proxy           string
	ProxyDomains    string
	LogLevel        string
	LangPath        string
	ProwlarrURL     string
	ProwlarrAPIKey  string

	DownloadSettings DownloadConfig
	SecuritySettings SecurityConfig
	Aria2Settings    Aria2Config
	VideoSettings    VideoConfig
}

// DownloadConfig конфигурация загрузок
type DownloadConfig struct {
	MaxConcurrentDownloads int
	DownloadTimeout        time.Duration
	ProgressUpdateInterval time.Duration
}

// SecurityConfig конфигурация безопасности
type SecurityConfig struct {
	PasswordMinLength int
}

// Aria2Config конфигурация Aria2
type Aria2Config struct {
	MaxPeers                 int
	MaxConnectionsPerServer  int
	Split                    int
	MinSplitSize             string
	BTMaxPeers               int
	BTRequestPeerSpeedLimit  string
	BTMaxOpenFiles           int
	MaxOverallUploadLimit    string
	MaxUploadLimit           string
	SeedRatio                float64
	SeedTime                 int
	BTTrackerTimeout         int
	BTTrackerInterval        int
	EnableDHT                bool
	EnablePeerExchange       bool
	EnableLocalPeerDiscovery bool
	FollowTorrent            bool
	ListenPort               string
	DHTPorts                 string
	BTSaveMetadata           bool
	BTHashCheckSeed          bool
	BTRequireCrypto          bool
	BTMinCryptoLevel         string
	CheckIntegrity           bool
	ContinueDownload         bool
	RemoteTime               bool
	FileAllocation           string
	HTTPProxy                string
	AllProxy                 string
	UserAgent                string
	Timeout                  int
	MaxTries                 int
	RetryWait                int
}

// VideoConfig конфигурация видео
type VideoConfig struct {
	EnableReencoding  bool
	ForceReencoding   bool
	VideoCodec        string
	AudioCodec        string
	OutputFormat      string
	FFmpegExtraArgs   string
	QualitySelector   string
	CompatibilityMode bool
}

// GetDownloadSettings возвращает настройки загрузки
func (c *Config) GetDownloadSettings() DownloadConfig {
	return c.DownloadSettings
}

// GetSecuritySettings возвращает настройки безопасности
func (c *Config) GetSecuritySettings() SecurityConfig {
	return c.SecuritySettings
}

// GetAria2Settings возвращает настройки Aria2
func (c *Config) GetAria2Settings() Aria2Config {
	return c.Aria2Settings
}

// GetVideoSettings возвращает настройки видео
func (c *Config) GetVideoSettings() VideoConfig {
	return c.VideoSettings
}
