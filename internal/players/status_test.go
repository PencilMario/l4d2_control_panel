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
