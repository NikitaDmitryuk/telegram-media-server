package utils

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"regexp"

	"github.com/sirupsen/logrus"
)

func SanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^а-яА-Яa-zA-Z0-9]+`)
	sanitized := re.ReplaceAllString(name, "_")
	logrus.WithFields(logrus.Fields{
		"original_name":  name,
		"sanitized_name": sanitized,
	}).Debug("Sanitizing file name")
	return sanitized
}

func LogAndReturnError(message string, err error) error {
	logrus.WithError(err).Error(message)
	return fmt.Errorf("%s: %v", message, err)
}

func IsValidLink(text string) bool {
	parsedURL, err := url.ParseRequestURI(text)
	if err != nil {
		logrus.WithError(err).Warn("Invalid URL format")
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		logrus.WithField("scheme", parsedURL.Scheme).Warn("Invalid URL scheme")
		return false
	}

	re := regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	isValid := re.MatchString(parsedURL.Host)
	logrus.WithFields(logrus.Fields{
		"url":      text,
		"is_valid": isValid,
	}).Debug("Validating URL")
	return isValid
}

func GenerateFileName(title string) string {
	fileName := SanitizeFileName(title) + ".mp4"
	logrus.WithFields(logrus.Fields{
		"title":     title,
		"file_name": fileName,
	}).Info("Generating file name")
	return fileName
}

func ValidateDurationString(durationStr string) (time.Duration, error) {
	re := regexp.MustCompile(`^(\d+)([hmd])$`)
	matches := re.FindStringSubmatch(durationStr)
	if len(matches) != 3 {
		return 0, errors.New("invalid duration format, expected format like '3h', '30m', '1d'")
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, errors.New("invalid numeric value in duration string")
	}

	unit := matches[2]
	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, errors.New("invalid time unit in duration string, expected 'h', 'm', or 'd'")
	}
}
