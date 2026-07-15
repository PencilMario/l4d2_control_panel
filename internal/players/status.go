package players

import (
	"regexp"
	"strconv"
	"strings"
)

type MatchInfo struct {
	Hostname       string `json:"hostname"`
	Version        string `json:"version"`
	Secure         *bool  `json:"secure"`
	OS             string `json:"os"`
	Map            string `json:"map"`
	PrivateAddress string `json:"private_address"`
	PublicAddress  string `json:"public_address"`
	Humans         int    `json:"humans"`
	MaxPlayers     int    `json:"max_players"`
}

type StatusPlayer struct {
	UserID    int    `json:"user_id"`
	Name      string `json:"name"`
	SteamID   string `json:"steam_id"`
	Connected string `json:"connected"`
	Ping      int    `json:"ping"`
	Loss      int    `json:"loss"`
}

type StatusSnapshot struct {
	Match   MatchInfo
	Players []StatusPlayer
}

var (
	hostnameLine = regexp.MustCompile(`(?m)^hostname\s*:\s*(.*?)\s*$`)
	versionLine  = regexp.MustCompile(`(?m)^version\s*:\s*(.*?)\s+(secure|insecure)(?:\s|$)`)
	addressLine  = regexp.MustCompile(`(?m)^udp/ip\s*:\s*(\S+)(?:\s+\[\s*public\s+(\S+)\s*\])?`)
	osLine       = regexp.MustCompile(`(?m)^os\s*:\s*(.*?)\s*$`)
	mapLine      = regexp.MustCompile(`(?m)^map\s*:\s*(.*?)\s*$`)
	playersLine  = regexp.MustCompile(`(?m)^players\s*:\s*(\d+)\s+humans.*\((\d+)\s+max\)`)
	statusLine   = regexp.MustCompile(`(?m)^#\s+(\d+)(?:\s+\d+)?\s+"([^"]+)"\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d+)\s+`)
)

func ParseStatusSnapshot(raw string) StatusSnapshot {
	result := StatusSnapshot{Players: []StatusPlayer{}}
	result.Match.Hostname = first(raw, hostnameLine, 1)
	result.Match.OS = first(raw, osLine, 1)
	result.Match.Map = first(raw, mapLine, 1)
	if match := versionLine.FindStringSubmatch(raw); len(match) != 0 {
		result.Match.Version = strings.TrimSpace(match[1])
		secure := match[2] == "secure"
		result.Match.Secure = &secure
	}
	if match := addressLine.FindStringSubmatch(raw); len(match) != 0 {
		result.Match.PrivateAddress = match[1]
		result.Match.PublicAddress = match[2]
	}
	if match := playersLine.FindStringSubmatch(raw); len(match) != 0 {
		result.Match.Humans, _ = strconv.Atoi(match[1])
		result.Match.MaxPlayers, _ = strconv.Atoi(match[2])
	}
	for _, match := range statusLine.FindAllStringSubmatch(raw, -1) {
		if match[3] == "BOT" {
			continue
		}
		userID, userErr := strconv.Atoi(match[1])
		ping, pingErr := strconv.Atoi(match[5])
		loss, lossErr := strconv.Atoi(match[6])
		if userErr != nil || pingErr != nil || lossErr != nil {
			continue
		}
		result.Players = append(result.Players, StatusPlayer{UserID: userID, Name: match[2], SteamID: match[3], Connected: match[4], Ping: ping, Loss: loss})
	}
	return result
}

func ParseStatus(raw string) []StatusPlayer {
	return ParseStatusSnapshot(raw).Players
}

func first(raw string, pattern *regexp.Regexp, group int) string {
	match := pattern.FindStringSubmatch(raw)
	if len(match) <= group {
		return ""
	}
	return strings.TrimSpace(match[group])
}
