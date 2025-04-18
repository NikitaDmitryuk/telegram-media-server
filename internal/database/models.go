package database

import "time"

type Movie struct {
	ID                   uint        `gorm:"primaryKey"`
	Name                 string      `gorm:"not null"`
	DownloadedPercentage int         `gorm:"not null;default:0;check:downloaded_percentage >= 0 AND downloaded_percentage <= 100"`
	Files                []MovieFile `gorm:"foreignKey:MovieID"`
}

type MovieFile struct {
	ID       uint   `gorm:"primaryKey"`
	MovieID  uint   `gorm:"not null"`
	FilePath string `gorm:"not null"`
	TempFile bool   `gorm:"not null"`
}

type UserRole string

const (
	AdminRole     UserRole = "admin"
	RegularRole   UserRole = "regular"
	TemporaryRole UserRole = "temporary"
)

type TemporaryPassword struct {
	ID        uint      `gorm:"primaryKey"`
	Password  string    `gorm:"not null;unique"`
	ExpiresAt time.Time `gorm:"not null"`
	Users     []User    `gorm:"many2many:user_temporary_passwords;"`
}

type User struct {
	ID        uint                `gorm:"primaryKey"`
	Name      string              `gorm:"not null"`
	ChatID    int64               `gorm:"not null"`
	Role      UserRole            `gorm:"not null;default:'regular'"`
	ExpiresAt *time.Time          `gorm:""`
	Passwords []TemporaryPassword `gorm:"many2many:user_temporary_passwords;"`
}

type DownloadHistory struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null"`
	MovieID   uint      `gorm:"not null"`
	Timestamp time.Time `gorm:"not null;autoCreateTime"`
	User      User      `gorm:"foreignKey:UserID"`
	Movie     Movie     `gorm:"foreignKey:MovieID"`
}
