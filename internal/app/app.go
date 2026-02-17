package app

import (
	tmsbot "github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	tmsdmanager "github.com/NikitaDmitryuk/telegram-media-server/internal/downloader/manager"
)

// App holds all shared application dependencies.
// Handlers receive *App instead of individual parameters.
type App struct {
	Bot             tmsbot.Service
	DB              database.Database
	Config          *config.Config
	DownloadManager tmsdmanager.Service
}
