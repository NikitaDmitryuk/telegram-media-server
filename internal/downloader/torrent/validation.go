package aria2

import (
	"fmt"
	"strings"
)

const (
	HeaderSize           = 512
	MinTorrentSize       = 20
	MaxTorrentSize       = 10 * 1024 * 1024
	MinBencodeDataLength = 10
)

func ValidateContent(data []byte, length int) error {
	content := string(data[:length])

	if isClearlyHTML(content) {
		return fmt.Errorf("file appears to be HTML, not a torrent file")
	}

	if isMagnetLink(content) {
		return fmt.Errorf("file appears to be a magnet link, not a torrent file")
	}

	if hasValidBencodeStructure(data, length) {
		return nil
	}

	return fmt.Errorf("invalid torrent file format (no valid bencode structure found)")
}

func isClearlyHTML(content string) bool {
	lowerContent := strings.ToLower(content)

	htmlIndicators := []string{
		"<!doctype html",
		"<html",
		"<head>",
		"<body>",
		"<title>",
		"<meta",
		"<script",
		"<style",
	}

	htmlCount := 0
	for _, indicator := range htmlIndicators {
		if strings.Contains(lowerContent, indicator) {
			htmlCount++
		}
	}

	if content[0] == '<' && htmlCount > 0 {
		return true
	}

	return htmlCount >= 2
}

func isMagnetLink(content string) bool {
	lowerContent := strings.ToLower(content)

	if strings.HasPrefix(lowerContent, "magnet:") {
		return true
	}

	if strings.Contains(lowerContent, "magnet:?xt=urn:btih:") {
		return true
	}

	return false
}

func hasValidBencodeStructure(data []byte, length int) bool {
	content := string(data[:length])

	requiredFields := []string{"announce", "info"}
	optionalFields := []string{"piece length", "pieces", "name", "length", "files"}

	foundRequired := 0
	foundOptional := 0

	for _, field := range requiredFields {
		if strings.Contains(content, field) {
			foundRequired++
		}
	}

	for _, field := range optionalFields {
		if strings.Contains(content, field) {
			foundOptional++
		}
	}

	if foundRequired >= 1 && foundOptional >= 1 {
		return true
	}

	for i := range length {
		if data[i] == 'd' {
			remaining := data[i:]
			if seemsLikeBencode(remaining) {
				return true
			}
		}
	}

	return false
}

func seemsLikeBencode(data []byte) bool {
	if len(data) < MinBencodeDataLength {
		return false
	}

	if data[0] != 'd' {
		return false
	}

	bencodePatterns := 0
	dataStr := string(data)

	if strings.Contains(dataStr, ":") {
		bencodePatterns++
	}

	if hasNumberPattern(dataStr) {
		bencodePatterns++
	}

	if strings.Contains(dataStr, "e") {
		bencodePatterns++
	}

	return bencodePatterns >= 2
}

func hasNumberPattern(s string) bool {
	for i := range len(s) - 1 {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] == ':' {
			return true
		}
	}
	return false
}
