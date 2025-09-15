package database

import "github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"

type Movie = domain.Movie
type MovieFile = domain.MovieFile
type UserRole = domain.UserRole
type TemporaryPassword = domain.TemporaryPassword
type User = domain.User

const (
	AdminRole     = domain.AdminRole
	RegularRole   = domain.RegularRole
	TemporaryRole = domain.TemporaryRole
)
