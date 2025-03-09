package utils

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"syscall"
)

func HasEnoughSpace(path string, requiredSpace int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		log.Printf("Error getting filesystem stats: %v\n", err)
		return false
	}
	availableSpace := stat.Bavail * uint64(stat.Bsize)

	log.Printf("Required space: %d bytes\n", requiredSpace)
	log.Printf("Available space: %d bytes\n", availableSpace)

	return availableSpace >= uint64(requiredSpace)
}

func IsEmptyDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Failed to read directory %s: %v", dir, err)
		return false
	}

	return len(entries) == 0
}

func SanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^а-яА-Яa-zA-Z0-9]+`)
	return re.ReplaceAllString(name, "_")
}

func LogAndReturnError(message string, err error) error {
	log.Printf("%s: %v\n", message, err)
	return fmt.Errorf("%s: %v", message, err)
}

func IsValidLink(text string) bool {
	parsedURL, err := url.ParseRequestURI(text)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	re := regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(parsedURL.Host)
}
