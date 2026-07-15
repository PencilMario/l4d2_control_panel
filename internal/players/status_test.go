package players

import "testing"

func TestParseStatusMapsNamesToStableUserIDs(t *testing.T) {
	raw := `# userid name uniqueid connected ping loss state rate adr
# 12 "Coach" STEAM_1:1:42 01:30 45 0 active 30000 1.2.3.4:27005
# 18 "Name With Space" STEAM_1:0:99 00:20 55 0 active 30000 2.3.4.5:27005`
	players := ParseStatus(raw)
	if len(players) != 2 || players[0].UserID != 12 || players[0].Name != "Coach" || players[1].UserID != 18 || players[1].Name != "Name With Space" {
		t.Fatalf("players=%#v", players)
	}
}

func TestParseStatusAcceptsL4D2EntityColumnBeforePlayerName(t *testing.T) {
	raw := `# userid name uniqueid connected ping loss state rate adr
#  2 1 "Sir.P" STEAM_1:0:526095818 20:55 30 0 active 100000 100.106.239.85:27005`

	players := ParseStatus(raw)
	if len(players) != 1 || players[0].UserID != 2 || players[0].Name != "Sir.P" || players[0].SteamID != "STEAM_1:0:526095818" {
		t.Fatalf("players=%#v", players)
	}
}
