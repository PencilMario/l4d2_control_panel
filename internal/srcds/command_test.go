package srcds

import (
	"slices"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

func TestCommandAppendsValidatedExtraArguments(t *testing.T) {
	value := domain.Instance{
		GamePort:     27015,
		SourceTVPort: 27020,
		StartMap:     "c2m1_highway",
		GameMode:     "coop",
		Tickrate:     100,
		MaxPlayers:   8,
		ExtraArgs:    `-strictportbind +hostname "Night Coop"`,
	}
	got, err := Command(value)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"./srcds_run", "-game", "left4dead2", "-console", "-port", "27015", "-tickrate", "100", "+map", "c2m1_highway", "+mp_gamemode", "coop", "-maxplayers", "8", "+tv_enable", "1", "+tv_port", "27020", "-strictportbind", "+hostname", "Night Coop"}
	if !slices.Equal(got, want) {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestCommandOmitsSourceTVWhenPortIsZero(t *testing.T) {
	value := domain.Instance{GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8}
	got, err := Command(value)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got, "+tv_enable") || slices.Contains(got, "+tv_port") {
		t.Fatalf("command=%q", got)
	}
}

func TestParseExtraArgsRejectsPanelOwnedOptions(t *testing.T) {
	for _, raw := range []string{"-port 27016", "+map c1m1_hotel", "-tickrate=30", "+tv_port 27020", "-game left4dead2", "-console"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseExtraArgs(raw); err == nil {
				t.Fatalf("accepted %q", raw)
			}
		})
	}
}

func TestParseExtraArgsRejectsBrokenQuoting(t *testing.T) {
	if _, err := ParseExtraArgs(`+hostname "unterminated`); err == nil {
		t.Fatal("accepted invalid quoting")
	}
}
