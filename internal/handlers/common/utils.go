package common

import (
	"net/url"
	"regexp"
	"strings"
)

var linkCandidateRE = regexp.MustCompile(`(?i)(https?://\S+|magnet:\?\S+)`)

func IsValidLink(text string) bool {
	link, ok := ExtractLink(text)
	return ok && link == strings.TrimSpace(text)
}

func ExtractLink(text string) (string, bool) {
	for _, candidate := range linkCandidateRE.FindAllString(text, -1) {
		link := cleanLinkCandidate(candidate)
		if isSupportedLink(link) {
			return link, true
		}
	}
	return "", false
}

func cleanLinkCandidate(candidate string) string {
	link := strings.TrimSpace(candidate)
	link = strings.Trim(link, "<>\"'`“”‘’")
	link = strings.TrimRight(link, ".,;:!?)]}>\"'`“”‘’")
	return link
}

func isSupportedLink(link string) bool {
	parsed, err := url.Parse(link)
	if err != nil {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "magnet":
		return parsed.RawQuery != ""
	default:
		return false
	}
}

func IsTorrentFile(fileName string) bool {
	return strings.HasSuffix(fileName, ".torrent")
}
