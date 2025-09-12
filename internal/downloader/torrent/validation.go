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

	if content != "" && content[0] == '<' && htmlCount > 0 {
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

	// Look for bencode-specific torrent field patterns like "8:announce" or "4:info"
	torrentPatterns := []string{
		"8:announce", "9:announce", // announce field
		"4:info",          // info field
		"12:piece length", // piece length field
		"6:pieces",        // pieces field
		"4:name",          // name field in info
		"5:files",         // files field in info
	}

	foundPatterns := 0
	for _, pattern := range torrentPatterns {
		if strings.Contains(content, pattern) {
			foundPatterns++
		}
	}

	// At least one torrent-specific pattern should be found
	return foundPatterns >= 1
}
