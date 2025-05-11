package common

import (
	"regexp"
	"strings"
)

func IsValidLink(text string) bool {
	re := regexp.MustCompile(`^https?://\S+$`)
	return re.MatchString(text)
}

func IsTorrentFile(fileName string) bool {
	return strings.HasSuffix(fileName, ".torrent")
}
