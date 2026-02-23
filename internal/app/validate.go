package app

import (
	"context"
	"errors"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/filemanager"
)

var (
	ErrAlreadyExists  = errors.New("movie already exists")
	ErrNotEnoughSpace = errors.New("not enough space")
)

// ValidateDownloadStart checks that the download can be started: files are not already present
// and there is enough disk space. Call from both API and Telegram before StartDownload.
func ValidateDownloadStart(ctx context.Context, a *App, dl downloader.Downloader) error {
	mainFiles, tempFiles, err := dl.GetFiles()
	if err != nil {
		return err
	}
	allFiles := make([]string, 0, len(mainFiles)+len(tempFiles))
	allFiles = append(allFiles, mainFiles...)
	allFiles = append(allFiles, tempFiles...)
	exists, err := a.DB.MovieExistsFiles(ctx, allFiles)
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}
	fileSize, err := dl.GetFileSize()
	if err != nil {
		return err
	}
	if !filemanager.HasEnoughSpace(a.Config.MoviePath, fileSize) {
		return ErrNotEnoughSpace
	}
	return nil
}
