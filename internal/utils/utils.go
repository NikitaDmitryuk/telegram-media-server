package utils

import (
	"fmt"
	"net/url"

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
