package a2sdefense

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeConfigSortsPortsWithoutMutatingInput(t *testing.T) {
	input := Config{Version: APIVersion, Enabled: true, Ports: []int{27020, 27015}, Revision: 2}
	got, err := NormalizeConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Ports, []int{27015, 27020}) {
		t.Fatalf("ports=%v", got.Ports)
	}
	if !reflect.DeepEqual(input.Ports, []int{27020, 27015}) {
		t.Fatalf("input mutated: %v", input.Ports)
	}
}

func TestNormalizeConfigRejectsInvalidValues(t *testing.T) {
	tests := []Config{
		{Version: 2, Enabled: true, Ports: []int{27015}, Revision: 1},
		{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 0},
		{Version: APIVersion, Enabled: true, Ports: []int{0}, Revision: 1},
		{Version: APIVersion, Enabled: true, Ports: []int{65536}, Revision: 1},
		{Version: APIVersion, Enabled: true, Ports: []int{27015, 27015}, Revision: 1},
		{Version: APIVersion, Enabled: false, Ports: []int{27015}, Revision: 1},
	}
	for _, input := range tests {
		if _, err := NormalizeConfig(input); err == nil {
			t.Fatalf("accepted invalid config: %+v", input)
		}
	}
	if _, err := NormalizeConfig(Config{Version: APIVersion, Enabled: true, Revision: 1}); err != nil {
		t.Fatalf("enabled empty port set should be valid: %v", err)
	}
	if _, err := NormalizeConfig(Config{Version: APIVersion, Enabled: false, Revision: 1}); err != nil {
		t.Fatalf("disabled empty port set should be valid: %v", err)
	}
}

func TestBuildEnableRestoreContainsFixedDefensePolicy(t *testing.T) {
	rules, err := BuildEnableRestore(Config{Version: APIVersion, Enabled: true, Ports: []int{27020, 27015}, Revision: 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"*filter",
		":" + InputChain + " - [0:0]",
		":" + ClassifyChain + " - [0:0]",
		":" + DropChain + " - [0:0]",
		"--dports 27015,27020",
		"0x54000000",
		"0x55000000",
		"0x56000000",
		"0x57000000",
		"0x69000000",
		"--hashlimit-name L4D2_A2S_TOTAL",
		"--hashlimit-mode dstport",
		"--hashlimit-above 300/second",
		"--hashlimit-burst 600",
		"--hashlimit-mode srcip",
		"--set --name L4D2_A2S_ATTACKER",
		"--limit 5/minute",
		"--nflog-group 100",
		"--nflog-prefix \"L4D2_A2S_DROP\"",
		"-j DROP",
		"-j RETURN",
		"COMMIT",
	} {
		if !strings.Contains(rules, required) {
			t.Errorf("rules missing %q\n%s", required, rules)
		}
	}
	if strings.Contains(rules, "-F INPUT") || strings.Contains(rules, "-P INPUT") {
		t.Fatalf("rules modify INPUT ownership boundary:\n%s", rules)
	}
	mark := strings.Index(rules, "--set --name L4D2_A2S_ATTACKER")
	log := strings.Index(rules, "--nflog-prefix \"L4D2_A2S_DROP\"")
	drop := strings.LastIndex(rules, "-j DROP")
	if mark < 0 || log < mark || drop < log {
		t.Fatalf("drop path order mark=%d log=%d drop=%d\n%s", mark, log, drop, rules)
	}
	if strings.Contains(rules, "-j LOG") || strings.Contains(rules, "--log-prefix") {
		t.Fatalf("kernel LOG target remains:\n%s", rules)
	}
	if strings.Contains(rules, "--name "+RecentName+" --rsource -j DROP") {
		t.Fatalf("blacklist bypasses sampled drop chain:\n%s", rules)
	}
}

func TestBuildEnableRestoreUsesPerQueryLimits(t *testing.T) {
	rules, err := BuildEnableRestore(Config{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"L4D2_A2S_INFO --hashlimit-mode srcip --hashlimit-above 20/second --hashlimit-burst 40",
		"L4D2_A2S_PLAYER --hashlimit-mode srcip --hashlimit-above 10/second --hashlimit-burst 20",
		"L4D2_A2S_RULES --hashlimit-mode srcip --hashlimit-above 10/second --hashlimit-burst 20",
		"L4D2_A2S_CHALLENGE --hashlimit-mode srcip --hashlimit-above 20/second --hashlimit-burst 40",
		"L4D2_A2S_OTHER69 --hashlimit-mode srcip --hashlimit-above 5/second --hashlimit-burst 10",
	} {
		if !strings.Contains(rules, expected) {
			t.Errorf("rules missing per-query policy %q\n%s", expected, rules)
		}
	}
}

func TestBuildDisableRestoreRemovesOnlyProjectChains(t *testing.T) {
	rules := BuildDisableRestore()
	for _, chain := range []string{InputChain, ClassifyChain, DropChain} {
		if !strings.Contains(rules, chain) {
			t.Errorf("disable rules missing %s", chain)
		}
	}
	if strings.Contains(rules, "-F INPUT") || strings.Contains(rules, "-P INPUT") {
		t.Fatalf("disable rules modify INPUT ownership boundary:\n%s", rules)
	}
}

func TestBuildEnableRestoreChunksMoreThanFifteenProtectedPorts(t *testing.T) {
	ports := make([]int, 16)
	for index := range ports {
		ports[index] = 27000 + index
	}
	rules, err := BuildEnableRestore(Config{Version: APIVersion, Enabled: true, Ports: ports, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(rules, "-m multiport --dports") != 4 {
		t.Fatalf("expected blacklist and classify rules for two port chunks:\n%s", rules)
	}
	if strings.Contains(rules, "--dports "+joinPorts(ports)) {
		t.Fatal("generated a multiport rule with more than 15 ports")
	}
	if !strings.Contains(rules, "--dports 27000,27001,27002,27003,27004,27005,27006,27007,27008,27009,27010,27011,27012,27013,27014") || !strings.Contains(rules, "--dports 27015") {
		t.Fatalf("missing expected chunks:\n%s", rules)
	}
}
