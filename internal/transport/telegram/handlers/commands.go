package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/lang"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BaseCommand содержит общие поля для всех команд
type BaseCommand struct {
	ChatID   int64
	UserID   int64
	Username string
	Text     string
	Args     []string
}

// LoginCommand представляет команду авторизации
type LoginCommand struct {
	BaseCommand
	Password string
}

// NewLoginCommand создает новую команду авторизации
func NewLoginCommand(update *tgbotapi.Update) domain.CommandInterface {
	textFields := strings.Fields(update.Message.Text)
	password := ""
	if len(textFields) > 1 {
		password = textFields[1]
	}

	return &LoginCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
			Args:     textFields[1:],
		},
		Password: password,
	}
}

func (*LoginCommand) GetType() string {
	return "login"
}

func (c *LoginCommand) Validate() error {
	if c.Password == "" {
		return utils.NewAppError("password_required", "password is required for login", nil)
	}
	return nil
}

func (c *LoginCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	// Эта команда должна обрабатываться через handler
	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "login_command_processed",
	}, nil
}

// StartCommand представляет команду старта
type StartCommand struct {
	BaseCommand
}

func NewStartCommand(update *tgbotapi.Update) domain.CommandInterface {
	return &StartCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
		},
	}
}

func (*StartCommand) GetType() string {
	return "start"
}

func (*StartCommand) Validate() error {
	return nil
}

func (c *StartCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: lang.Translate("general.commands.start", nil),
	}, nil
}

// ListMoviesCommand представляет команду списка фильмов
type ListMoviesCommand struct {
	BaseCommand
}

func NewListMoviesCommand(update *tgbotapi.Update) domain.CommandInterface {
	return &ListMoviesCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
		},
	}
}

func (*ListMoviesCommand) GetType() string {
	return "ls"
}

func (*ListMoviesCommand) Validate() error {
	return nil
}

func (c *ListMoviesCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "list_movies_command_processed",
	}, nil
}

// DeleteMovieCommand представляет команду удаления фильма
type DeleteMovieCommand struct {
	BaseCommand
	MovieID uint
	All     bool
}

func NewDeleteMovieCommand(update *tgbotapi.Update) domain.CommandInterface {
	textFields := strings.Fields(update.Message.Text)

	cmd := &DeleteMovieCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
			Args:     textFields[1:],
		},
	}

	if len(textFields) > 1 {
		if textFields[1] == "all" {
			cmd.All = true
		} else {
			if movieID, err := strconv.ParseUint(textFields[1], 10, 32); err == nil {
				cmd.MovieID = uint(movieID)
			}
		}
	}

	return cmd
}

func (*DeleteMovieCommand) GetType() string {
	return "rm"
}

func (c *DeleteMovieCommand) Validate() error {
	if !c.All && c.MovieID == 0 {
		return utils.NewAppError("invalid_movie_id", "movie ID is required or use 'all'", nil)
	}
	return nil
}

func (c *DeleteMovieCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "delete_movie_command_processed",
	}, nil
}

// TempPasswordCommand представляет команду создания временного пароля
type TempPasswordCommand struct {
	BaseCommand
	Duration string
}

func NewTempPasswordCommand(update *tgbotapi.Update) domain.CommandInterface {
	textFields := strings.Fields(update.Message.Text)
	duration := ""
	if len(textFields) > 1 {
		duration = textFields[1]
	}

	return &TempPasswordCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
			Args:     textFields[1:],
		},
		Duration: duration,
	}
}

func (*TempPasswordCommand) GetType() string {
	return "temp"
}

func (c *TempPasswordCommand) Validate() error {
	if c.Duration == "" {
		return utils.NewAppError("duration_required", "duration is required for temp password", nil)
	}

	// Проверяем валидность формата длительности
	validDurations := []string{"1d", "3h", "30m", "1h", "2h", "6h", "12h"}
	for _, valid := range validDurations {
		if c.Duration == valid {
			return nil
		}
	}

	return utils.NewAppError("invalid_duration", "invalid duration format", map[string]any{
		"duration": c.Duration,
		"valid":    validDurations,
	})
}

func (c *TempPasswordCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "temp_password_command_processed",
	}, nil
}

// DownloadLinkCommand представляет команду загрузки по ссылке
type DownloadLinkCommand struct {
	BaseCommand
	Link string
}

func NewDownloadLinkCommand(update *tgbotapi.Update, link string) domain.CommandInterface {
	return &DownloadLinkCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
		},
		Link: link,
	}
}

func (*DownloadLinkCommand) GetType() string {
	return "download_link"
}

func (c *DownloadLinkCommand) Validate() error {
	if c.Link == "" {
		return utils.NewAppError("link_required", "download link is required", nil)
	}

	// Простая валидация URL
	if !strings.HasPrefix(c.Link, "http://") && !strings.HasPrefix(c.Link, "https://") {
		return utils.NewAppError("invalid_link", "link must start with http:// or https://", map[string]any{
			"link": c.Link,
		})
	}

	return nil
}

func (c *DownloadLinkCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	logger.Log.WithFields(map[string]any{
		"chat_id": c.ChatID,
		"link":    c.Link,
	}).Info("Processing download link command")

	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "download_link_command_processed",
	}, nil
}

// DownloadTorrentCommand представляет команду загрузки торрента
type DownloadTorrentCommand struct {
	BaseCommand
	FileName string
	FileData []byte
}

func NewDownloadTorrentCommand(update *tgbotapi.Update, fileName string, fileData []byte) domain.CommandInterface {
	return &DownloadTorrentCommand{
		BaseCommand: BaseCommand{
			ChatID:   update.Message.Chat.ID,
			UserID:   update.Message.From.ID,
			Username: update.Message.From.UserName,
			Text:     update.Message.Text,
		},
		FileName: fileName,
		FileData: fileData,
	}
}

func (*DownloadTorrentCommand) GetType() string {
	return "download_torrent"
}

func (c *DownloadTorrentCommand) Validate() error {
	if c.FileName == "" {
		return utils.NewAppError("filename_required", "torrent filename is required", nil)
	}

	if !strings.HasSuffix(c.FileName, ".torrent") {
		return utils.NewAppError("invalid_torrent", "file must have .torrent extension", map[string]any{
			"filename": c.FileName,
		})
	}

	if len(c.FileData) == 0 {
		return utils.NewAppError("empty_file", "torrent file data is empty", nil)
	}

	return nil
}

func (c *DownloadTorrentCommand) Execute(_ context.Context) (*domain.CommandResult, error) {
	logger.Log.WithFields(map[string]any{
		"chat_id":  c.ChatID,
		"filename": c.FileName,
		"size":     len(c.FileData),
	}).Info("Processing download torrent command")

	return &domain.CommandResult{
		ChatID:  c.ChatID,
		Message: "download_torrent_command_processed",
	}, nil
}
