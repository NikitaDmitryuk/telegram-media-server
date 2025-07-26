package database

import "github.com/NikitaDmitryuk/telegram-media-server/internal/models"

type Movie = models.Movie
type MovieFile = models.MovieFile
type UserRole = models.UserRole
type TemporaryPassword = models.TemporaryPassword
type User = models.User
type DownloadHistory = models.DownloadHistory

const (
	AdminRole     = models.AdminRole
	RegularRole   = models.RegularRole
	TemporaryRole = models.TemporaryRole
)
