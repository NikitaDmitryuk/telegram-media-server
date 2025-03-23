package database

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Movie struct {
	ID                   uint        `gorm:"primaryKey"`
	Name                 string      `gorm:"not null"`
	DownloadedPercentage int         `gorm:"not null;default:0;check:downloaded_percentage >= 0 AND downloaded_percentage <= 100"`
	Files                []MovieFile `gorm:"foreignKey:MovieID"`
}

func (m *Movie) BeforeCreate(tx *gorm.DB) (err error) {
	var smallestID uint
	err = tx.WithContext(context.Background()).
		Raw("SELECT COALESCE(MIN(id) + 1, 1) AS smallest_id FROM movies WHERE id + 1 NOT IN (SELECT id FROM movies)").
		Scan(&smallestID).Error
	if err != nil {
		logrus.WithError(err).Error("Failed to calculate smallest available ID")
		return err
	}
	m.ID = smallestID
	return nil
}

type MovieFile struct {
	ID       uint   `gorm:"primaryKey"`
	MovieID  uint   `gorm:"not null"`
	FilePath string `gorm:"not null"`
	TempFile bool   `gorm:"not null"`
}

type User struct {
	ID     uint   `gorm:"primaryKey"`
	Name   string `gorm:"not null"`
	ChatID int64  `gorm:"not null"`
}
