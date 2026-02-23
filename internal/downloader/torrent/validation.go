package aria2

import (
	"fmt"
	"os"
	"regexp"
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
		"13:announce-list", // announce-list field (alternative to announce)
		"4:info",           // info field
		"12:piece length",  // piece length field
		"6:pieces",         // pieces field
		"4:name",           // name field in info
		"5:files",          // files field in info
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

// ValidateTorrentFile validates a torrent file at the given path
func ValidateTorrentFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}

	return ValidateContent(data, len(data))
}

// btih hex length (20 bytes) and base32 length (32 chars)
const btihHexLen = 40
const btihHexLenShort = 24 // some indexes use 24-char hex
const btihBase32Len = 32

var btihHexRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)
var btihBase32Re = regexp.MustCompile(`^[2-7A-Za-z]+$`)

// endOfBtih returns true when s[i] starts a param delimiter: '&', '?', or percent-encoded %26, %3F.
func endOfBtih(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	if s[i] == '&' || s[i] == '?' {
		return true
	}
	if s[i] == '%' && i+2 < len(s) {
		hex := strings.ToLower(s[i+1 : i+3])
		if hex == "26" || hex == "3f" {
			return true
		}
	}
	return false
}

// ValidateMagnetBtih checks that the magnet URI contains a valid btih (40 hex or 32 base32 chars).
// Returns nil if valid or no btih present; error if btih is present but invalid length/format.
// Stops at &, ?, %26, %3F so URL-encoded magnet links are parsed correctly.
func ValidateMagnetBtih(magnet string) error {
	magnet = strings.TrimSpace(magnet)
	if !strings.HasPrefix(strings.ToLower(magnet), "magnet:") {
		return nil
	}
	idx := strings.Index(strings.ToLower(magnet), "urn:btih:")
	if idx == -1 {
		return nil // no btih, let aria2 handle
	}
	hashStart := idx + len("urn:btih:")
	hashEnd := hashStart
	for hashEnd < len(magnet) && !endOfBtih(magnet, hashEnd) {
		hashEnd++
	}
	hash := magnet[hashStart:hashEnd]
	switch len(hash) {
	case btihHexLen, btihHexLenShort:
		if !btihHexRe.MatchString(hash) {
			return fmt.Errorf("invalid magnet: btih must be 40 hex or 32 base32 characters")
		}
		return nil
	case btihBase32Len:
		if !btihBase32Re.MatchString(hash) {
			return fmt.Errorf("invalid magnet: btih must be 40 hex or 32 base32 characters")
		}
		return nil
	default:
		// Some clients send magnet without & after btih (e.g. whole tail as one token); accept if first 32 chars are valid base32
		if len(hash) >= btihBase32Len && btihBase32Re.MatchString(hash[:btihBase32Len]) {
			return nil
		}
		return fmt.Errorf("invalid magnet: btih length must be 24 or 40 (hex) or 32 (base32) characters, got %d", len(hash))
	}
}
