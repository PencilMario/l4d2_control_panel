package players

import (
	"regexp"
	"strconv"
)

type StatusPlayer struct {
	UserID  int    `json:"user_id"`
	Name    string `json:"name"`
	SteamID string `json:"steam_id"`
}

var statusLine = regexp.MustCompile(`(?m)^#\s+(\d+)\s+"([^"]+)"\s+(\S+)\s+`)

func ParseStatus(raw string) []StatusPlayer {
	matches := statusLine.FindAllStringSubmatch(raw, -1)
	result := make([]StatusPlayer, 0, len(matches))
	for _, match := range matches {
		id, err := strconv.Atoi(match[1])
		if err == nil {
			result = append(result, StatusPlayer{UserID: id, Name: match[2], SteamID: match[3]})
		}
	}
	return result
}
