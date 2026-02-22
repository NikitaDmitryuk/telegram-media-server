package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	MinPasswordLength = 8
)

func (c *Config) validate() error {
	if err := c.validateRequiredFields(); err != nil {
		return err
	}
	if err := c.validatePasswords(); err != nil {
		return err
	}
	if err := c.validateProwlarr(); err != nil {
		return err
	}
	if err := c.validateTMSAPI(); err != nil {
		return err
	}
	if err := c.validateDownloadSettings(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateRequiredFields() error {
	if c.BotToken == "" {
		return errors.New("BOT_TOKEN is required")
	}
	if c.MoviePath == "" {
		return errors.New("MOVIE_PATH is required")
	}
	if c.AdminPassword == "" {
		return errors.New("ADMIN_PASSWORD is required")
	}

	if _, err := os.Stat(c.MoviePath); os.IsNotExist(err) {
		return fmt.Errorf("MOVIE_PATH directory does not exist: %s", c.MoviePath)
	}

	return nil
}

func (c *Config) validatePasswords() error {
	if len(c.AdminPassword) < MinPasswordLength {
		return fmt.Errorf("ADMIN_PASSWORD must be at least %d characters long", MinPasswordLength)
	}

	if c.RegularPassword != "" && len(c.RegularPassword) < MinPasswordLength {
		return fmt.Errorf("REGULAR_PASSWORD must be at least %d characters long", MinPasswordLength)
	}

	if c.AdminPassword == c.RegularPassword && c.RegularPassword != "" {
		return errors.New("ADMIN_PASSWORD and REGULAR_PASSWORD cannot be the same")
	}

	return nil
}

func (c *Config) validateProwlarr() error {
	prowlarrConfigured := c.ProwlarrURL != "" || c.ProwlarrAPIKey != ""

	if prowlarrConfigured {
		if c.ProwlarrURL == "" {
			return errors.New("PROWLARR_URL is required when PROWLARR_API_KEY is set")
		}
		if c.ProwlarrAPIKey == "" {
			return errors.New("PROWLARR_API_KEY is required when PROWLARR_URL is set")
		}
	}

	return nil
}

func (c *Config) validateTMSAPI() error {
	if !c.TMSAPIEnabled {
		return nil
	}
	// TMS_API_KEY is optional: when empty, API accepts only requests from localhost
	host, port, err := net.SplitHostPort(c.TMSAPIListen)
	if err != nil {
		return fmt.Errorf("TMS_API_LISTEN must be host:port or :port: %w", err)
	}
	if port == "" {
		return errors.New("TMS_API_LISTEN must include a port")
	}
	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p > 65535 {
		return errors.New("TMS_API_LISTEN port must be between 1 and 65535")
	}
	_ = host // host may be empty for ":8080"
	return nil
}

func (c *Config) validateDownloadSettings() error {
	if c.DownloadSettings.MaxConcurrentDownloads <= 0 {
		return errors.New("MAX_CONCURRENT_DOWNLOADS must be greater than 0")
	}

	if c.DownloadSettings.DownloadTimeout < 0 {
		return errors.New("DOWNLOAD_TIMEOUT cannot be negative")
	}

	return nil
}
