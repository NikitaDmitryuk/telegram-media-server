package models

import "time"

type Movie struct {
	ID                   uint        `json:"id" gorm:"primaryKey"`
	Name                 string      `json:"name" gorm:"not null"`
	DownloadedPercentage int         `json:"downloaded_percentage" gorm:"not null;default:0"`
	Files                []MovieFile `json:"files" gorm:"foreignKey:MovieID"`
	CreatedAt            time.Time   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            time.Time   `json:"updated_at" gorm:"autoUpdateTime"`
}

type MovieFile struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	MovieID   uint      `json:"movie_id" gorm:"not null;constraint:OnDelete:CASCADE;"`
	FilePath  string    `json:"file_path" gorm:"not null"`
	TempFile  bool      `json:"temp_file" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

type UserRole string

const (
	AdminRole     UserRole = "admin"
	RegularRole   UserRole = "regular"
	TemporaryRole UserRole = "temporary"
)

func (r UserRole) String() string {
	return string(r)
}

func (r UserRole) IsValid() bool {
	switch r {
	case AdminRole, RegularRole, TemporaryRole:
		return true
	default:
		return false
	}
}

func (r UserRole) HasPermission(action string) bool {
	switch action {
	case "download":
		return r == AdminRole || r == RegularRole || r == TemporaryRole
	case "delete":
		return r == AdminRole || r == RegularRole
	case "manage_users":
		return r == AdminRole
	case "generate_temp_password":
		return r == AdminRole
	default:
		return false
	}
}

type TemporaryPassword struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Password  string    `json:"password" gorm:"not null;unique"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	Users     []User    `json:"users" gorm:"many2many:user_temporary_passwords;"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (tp *TemporaryPassword) IsExpired() bool {
	return time.Now().After(tp.ExpiresAt)
}

type User struct {
	ID        uint                `json:"id" gorm:"primaryKey"`
	Name      string              `json:"name" gorm:"not null"`
	ChatID    int64               `json:"chat_id" gorm:"not null"`
	Role      UserRole            `json:"role" gorm:"not null;default:'regular'"`
	ExpiresAt *time.Time          `json:"expires_at" gorm:""`
	Passwords []TemporaryPassword `json:"passwords" gorm:"many2many:user_temporary_passwords;"`
	CreatedAt time.Time           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time           `json:"updated_at" gorm:"autoUpdateTime"`
}

func (u *User) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*u.ExpiresAt)
}

func (u *User) IsActive() bool {
	return !u.IsExpired()
}

type DownloadHistory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null"`
	MovieID   uint      `json:"movie_id" gorm:"not null"`
	Timestamp time.Time `json:"timestamp" gorm:"not null;autoCreateTime"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	Movie     Movie     `json:"movie" gorm:"foreignKey:MovieID"`
}

type DownloadStatus struct {
	MovieID                uint          `json:"movie_id"`
	Title                  string        `json:"title"`
	Progress               float64       `json:"progress"`
	IsActive               bool          `json:"is_active"`
	EstimatedTimeRemaining time.Duration `json:"estimated_time_remaining,omitempty"`
	DownloadSpeed          string        `json:"download_speed,omitempty"`
	Error                  string        `json:"error,omitempty"`
}

type SearchSession struct {
	ChatID     int64                 `json:"chat_id"`
	Query      string                `json:"query"`
	Stage      string                `json:"stage"`
	Offset     int                   `json:"offset"`
	MessageIDs []int                 `json:"message_ids"`
	Results    []TorrentSearchResult `json:"results"`
	CreatedAt  time.Time             `json:"created_at"`
}

type TorrentSearchResult struct {
	Title       string `json:"title"`
	Size        int64  `json:"size"`
	Magnet      string `json:"magnet"`
	TorrentURL  string `json:"torrent_url"`
	IndexerName string `json:"indexer_name"`
	InfoHash    string `json:"info_hash"`
	Peers       int    `json:"peers"`
}
