package databaseconfig

import (
	"fmt"
	"strings"
)

func quote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func Render(config Config) ([]byte, error) {
	config = Normalize(config)
	if err := Validate(config); err != nil {
		return nil, err
	}
	var output strings.Builder
	output.WriteString("\"Databases\"\n{\n")
	fmt.Fprintf(&output, "\t\"driver_default\"\t\t%s\n", quote(config.DefaultDriver))
	for _, connection := range config.Connections {
		fmt.Fprintf(&output, "\n\t%s\n\t{\n", quote(connection.Name))
		fields := [][2]string{{"driver", connection.Driver}, {"host", connection.Host}, {"database", connection.Database}, {"user", connection.User}, {"pass", connection.Password}, {"timeout", connection.Timeout}, {"port", connection.Port}}
		for _, field := range fields {
			if field[1] == "" && field[0] != "pass" {
				continue
			}
			fmt.Fprintf(&output, "\t\t%s\t\t\t%s\n", quote(field[0]), quote(field[1]))
		}
		output.WriteString("\t}\n")
	}
	output.WriteString("}\n")
	return []byte(output.String()), nil
}
