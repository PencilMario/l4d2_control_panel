package a2sdefense

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	InputChain    = "L4D2_A2S_DEFENSE"
	SlotAChain    = "L4D2_A2S_SLOT_A"
	SlotBChain    = "L4D2_A2S_SLOT_B"
	ClassifyChain = "L4D2_A2S_CLASS_A"
	DropChain     = "L4D2_A2S_DROP_A"
	ClassifyB     = "L4D2_A2S_CLASS_B"
	DropB         = "L4D2_A2S_DROP_B"
	RecentName    = "L4D2_A2S_ATTACKER"
)

type ruleSlot struct {
	entry    string
	classify string
	drop     string
}

var slotA = ruleSlot{entry: SlotAChain, classify: ClassifyChain, drop: DropChain}
var slotB = ruleSlot{entry: SlotBChain, classify: ClassifyB, drop: DropB}

type queryPolicy struct {
	opcode byte
	name   string
	rate   int
	burst  int
}

var fixedQueryPolicies = []queryPolicy{
	{opcode: 0x54, name: "INFO", rate: 20, burst: 40},
	{opcode: 0x55, name: "PLAYER", rate: 10, burst: 20},
	{opcode: 0x56, name: "RULES", rate: 10, burst: 20},
	{opcode: 0x57, name: "CHALLENGE", rate: 20, burst: 40},
	{opcode: 0x69, name: "OTHER69", rate: 5, burst: 10},
}

func BuildEnableRestore(input Config) (string, error) {
	return buildEnableRestore(input, slotA)
}

func buildEnableRestore(input Config, active ruleSlot) (string, error) {
	config, err := NormalizeConfig(input)
	if err != nil || !config.Enabled {
		return "", ErrInvalidConfig
	}
	var rules strings.Builder
	rules.WriteString("*filter\n")
	for _, chain := range projectChains() {
		fmt.Fprintf(&rules, ":%s - [0:0]\n", chain)
	}
	fmt.Fprintf(&rules, "-F %s\n-F %s\n-F %s\n-F %s\n", InputChain, active.entry, active.classify, active.drop)
	fmt.Fprintf(&rules, "-A %s -j %s\n-A %s -j RETURN\n", InputChain, active.entry, InputChain)
	if len(config.Ports) == 0 {
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", active.entry)
	} else {
		ports := joinPorts(config.Ports)
		fmt.Fprintf(&rules, "-A %s -p udp -m multiport --dports %s -m recent --update --seconds 60 --hitcount 3 --name %s --rsource -j DROP\n", active.entry, ports, RecentName)
		fmt.Fprintf(&rules, "-A %s -p udp -m multiport --dports %s -j %s\n", active.entry, ports, active.classify)
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", active.entry)

		fmt.Fprintf(&rules, "-A %s -m u32 --u32 \"0>>22&0x3C@8=0xFFFFFFFF && 0>>22&0x3C@12&0xFF000000=0x54000000:0x57000000,0x69000000\" -m hashlimit --hashlimit-name L4D2_A2S_TOTAL --hashlimit-mode dstport --hashlimit-above 300/second --hashlimit-burst 600 --hashlimit-htable-expire 60000 -j %s\n", active.classify, active.drop)
		for _, policy := range fixedQueryPolicies {
			fmt.Fprintf(&rules, "-A %s -m u32 --u32 \"0>>22&0x3C@8=0xFFFFFFFF && 0>>22&0x3C@12&0xFF000000=0x%02X000000\" -m hashlimit --hashlimit-name L4D2_A2S_%s --hashlimit-mode srcip --hashlimit-above %d/second --hashlimit-burst %d --hashlimit-htable-expire 60000 -j %s\n", active.classify, policy.opcode, policy.name, policy.rate, policy.burst, active.drop)
		}
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", active.classify)
	}
	fmt.Fprintf(&rules, "-A %s -m recent --set --name %s --rsource\n", active.drop, RecentName)
	fmt.Fprintf(&rules, "-A %s -m limit --limit 5/minute --limit-burst 5 -j LOG --log-prefix \"L4D2_A2S_DROP: \" --log-level 4\n", active.drop)
	fmt.Fprintf(&rules, "-A %s -j DROP\nCOMMIT\n", active.drop)
	return rules.String(), nil
}

func BuildDisableRestore() string {
	var rules strings.Builder
	rules.WriteString("*filter\n")
	for _, chain := range projectChains() {
		fmt.Fprintf(&rules, ":%s - [0:0]\n-F %s\n", chain, chain)
	}
	for index := len(projectChains()) - 1; index >= 0; index-- {
		fmt.Fprintf(&rules, "-X %s\n", projectChains()[index])
	}
	rules.WriteString("COMMIT\n")
	return rules.String()
}

func projectChains() []string {
	return []string{InputChain, SlotAChain, ClassifyChain, DropChain, SlotBChain, ClassifyB, DropB}
}

func joinPorts(ports []int) string {
	values := make([]string, len(ports))
	for index, port := range ports {
		values[index] = strconv.Itoa(port)
	}
	return strings.Join(values, ",")
}
