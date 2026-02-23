package common

import (
	"regexp"
	"strings"
)

func IsValidLink(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(strings.ToLower(text), "magnet:") {
		return true
	}
	re := regexp.MustCompile(`^https?://\S+$`)
	return re.MatchString(text)
}

func IsTorrentFile(fileName string) bool {
	return strings.HasSuffix(fileName, ".torrent")
}
