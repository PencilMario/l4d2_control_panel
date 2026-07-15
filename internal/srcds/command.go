package srcds

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

var reservedOptions = map[string]struct{}{
	"-game":        {},
	"-console":     {},
	"-port":        {},
	"-tickrate":    {},
	"+map":         {},
	"+mp_gamemode": {},
	"-maxplayers":  {},
	"+tv_enable":   {},
	"+tv_port":     {},
}

func ParseExtraArgs(raw string) ([]string, error) {
	args, err := shellwords.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid extra arguments: %w", err)
	}
	for _, arg := range args {
		key := strings.SplitN(arg, "=", 2)[0]
		if _, blocked := reservedOptions[key]; blocked {
			return nil, fmt.Errorf("%s is managed by the Panel", key)
		}
	}
	if args == nil {
		args = []string{}
	}
	return args, nil
}

func Command(value domain.Instance) ([]string, error) {
	args := []string{
		"./srcds_run",
		"-game", "left4dead2",
		"-console",
		"-port", strconv.Itoa(value.GamePort),
		"-tickrate", strconv.Itoa(value.Tickrate),
		"+map", value.StartMap,
		"+mp_gamemode", value.GameMode,
		"-maxplayers", strconv.Itoa(value.MaxPlayers),
	}
	if value.SourceTVPort != 0 {
		args = append(args, "+tv_enable", "1", "+tv_port", strconv.Itoa(value.SourceTVPort))
	}
	extra, err := ParseExtraArgs(value.ExtraArgs)
	if err != nil {
		return nil, err
	}
	return append(args, extra...), nil
}
