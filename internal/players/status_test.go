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

func TestParseStatusSnapshotReadsMatchAndHumanOperations(t *testing.T) {
	raw := `hostname: 6
version : 2.2.4.3 10097 secure  (unknown)
udp/ip  : 127.0.1.1:27991 [ public 221.215.78.153:27991 ]
os      : Linux Dedicated
map     : c2m1_highway
players : 1 humans, 0 bots (12 max) (not hibernating) (unreserved)

# userid name uniqueid connected ping loss state rate
#  2 1 "Sir.P" STEAM_1:0:526095818 00:48 29 0 active 100000
# 3 "Rochelle" BOT active
#end`

	snapshot := ParseStatusSnapshot(raw)
	if snapshot.Match.Hostname != "6" || snapshot.Match.Version != "2.2.4.3 10097" || snapshot.Match.Secure == nil || !*snapshot.Match.Secure || snapshot.Match.PrivateAddress != "127.0.1.1:27991" || snapshot.Match.PublicAddress != "221.215.78.153:27991" || snapshot.Match.OS != "Linux Dedicated" || snapshot.Match.Map != "c2m1_highway" || snapshot.Match.Humans != 1 || snapshot.Match.MaxPlayers != 12 {
		t.Fatalf("match=%#v", snapshot.Match)
	}
	if len(snapshot.Players) != 1 {
		t.Fatalf("players=%#v", snapshot.Players)
	}
	player := snapshot.Players[0]
	if player.UserID != 2 || player.Name != "Sir.P" || player.SteamID != "STEAM_1:0:526095818" || player.Connected != "00:48" || player.Ping != 29 || player.Loss != 0 {
		t.Fatalf("player=%#v", player)
	}
}
