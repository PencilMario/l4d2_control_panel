package a2sdefense

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	InputChain    = "L4D2_A2S_DEFENSE"
	ClassifyChain = "L4D2_A2S_CLASSIFY"
	DropChain     = "L4D2_A2S_DROP"
	RecentName    = "L4D2_A2S_ATTACKER"
)

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
	config, err := NormalizeConfig(input)
	if err != nil || !config.Enabled {
		return "", ErrInvalidConfig
	}
	var rules strings.Builder
	rules.WriteString("*filter\n")
	for _, chain := range []string{InputChain, ClassifyChain, DropChain} {
		fmt.Fprintf(&rules, ":%s - [0:0]\n-F %s\n", chain, chain)
	}
	if len(config.Ports) == 0 {
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", InputChain)
	} else {
		ports := joinPorts(config.Ports)
		fmt.Fprintf(&rules, "-A %s -p udp -m multiport --dports %s -m recent --update --seconds 60 --hitcount 3 --name %s --rsource -j DROP\n", InputChain, ports, RecentName)
		fmt.Fprintf(&rules, "-A %s -p udp -m multiport --dports %s -j %s\n", InputChain, ports, ClassifyChain)
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", InputChain)

		fmt.Fprintf(&rules, "-A %s -m u32 --u32 \"0>>22&0x3C@8=0xFFFFFFFF && 0>>22&0x3C@12&0xFF000000=0x54000000:0x57000000,0x69000000\" -m hashlimit --hashlimit-name L4D2_A2S_TOTAL --hashlimit-mode dstport --hashlimit-above 300/second --hashlimit-burst 600 --hashlimit-htable-expire 60000 -j %s\n", ClassifyChain, DropChain)
		for _, policy := range fixedQueryPolicies {
			fmt.Fprintf(&rules, "-A %s -m u32 --u32 \"0>>22&0x3C@8=0xFFFFFFFF && 0>>22&0x3C@12&0xFF000000=0x%02X000000\" -m hashlimit --hashlimit-name L4D2_A2S_%s --hashlimit-mode srcip --hashlimit-above %d/second --hashlimit-burst %d --hashlimit-htable-expire 60000 -j %s\n", ClassifyChain, policy.opcode, policy.name, policy.rate, policy.burst, DropChain)
		}
		fmt.Fprintf(&rules, "-A %s -j RETURN\n", ClassifyChain)
	}
	fmt.Fprintf(&rules, "-A %s -m recent --set --name %s --rsource\n", DropChain, RecentName)
	fmt.Fprintf(&rules, "-A %s -m limit --limit 5/minute --limit-burst 5 -j LOG --log-prefix \"L4D2_A2S_DROP: \" --log-level 4\n", DropChain)
	fmt.Fprintf(&rules, "-A %s -j DROP\nCOMMIT\n", DropChain)
	return rules.String(), nil
}

func BuildDisableRestore() string {
	var rules strings.Builder
	rules.WriteString("*filter\n")
	for _, chain := range []string{InputChain, ClassifyChain, DropChain} {
		fmt.Fprintf(&rules, ":%s - [0:0]\n-F %s\n", chain, chain)
	}
	rules.WriteString("COMMIT\n")
	return rules.String()
}

func joinPorts(ports []int) string {
	values := make([]string, len(ports))
	for index, port := range ports {
		values[index] = strconv.Itoa(port)
	}
	return strings.Join(values, ",")
}
