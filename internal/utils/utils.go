package utils

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"syscall"

	"github.com/sirupsen/logrus"
)

func HasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		logrus.WithError(err).Error("Failed to get filesystem stats")
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	logrus.WithFields(logrus.Fields{
		"required_space":  requiredSpace,
		"available_space": availableSpace,
	}).Info("Checking available disk space")

	return availableSpace >= uint64(requiredSpace)
}

func IsEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to read directory: %s", dir)
		return false
	}

	logrus.WithField("directory", dir).Info("Checking if directory is empty")
	return len(entries) == 0
}

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
