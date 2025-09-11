package database

import "github.com/NikitaDmitryuk/telegram-media-server/internal/models"

type Movie = models.Movie
type MovieFile = models.MovieFile
type UserRole = models.UserRole
type TemporaryPassword = models.TemporaryPassword
type User = models.User

const (
	AdminRole     = models.AdminRole
	RegularRole   = models.RegularRole
	TemporaryRole = models.TemporaryRole
)
