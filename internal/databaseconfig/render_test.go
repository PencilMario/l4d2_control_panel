package databaseconfig

import (
	"strings"
	"testing"
)

func TestDatabaseDefaultsMatchSourceModExample(t *testing.T) {
	config := Defaults()
	if config.DefaultDriver != "mysql" || len(config.Connections) != 3 {
		t.Fatalf("defaults=%+v", config)
	}
	if config.Connections[0].Name != "default" || config.Connections[2].Database != "clientprefs-sqlite" {
		t.Fatalf("connections=%+v", config.Connections)
	}
}

func TestDatabaseRenderOmitsSQLiteLocalPortAndTimeout(t *testing.T) {
	config := Defaults()
	config.Connections[2].Port = "3306"
	config.Connections[2].Timeout = "5"
	raw, err := Render(config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	block := text[strings.Index(text, `"clientprefs"`):]
	if strings.Contains(block, `"port"`) || strings.Contains(block, `"timeout"`) {
		t.Fatalf("sqlite block contains ignored fields:\n%s", block)
	}
}

func TestDatabaseValidationRejectsDuplicateNames(t *testing.T) {
	config := Defaults()
	config.Connections[1].Name = "default"
	if err := Validate(config); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestDatabaseRenderEscapesValues(t *testing.T) {
	config := Defaults()
	config.Connections[0].Password = `a\"b`
	raw, err := Render(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"a\\\"b"`) {
		t.Fatalf("render=%s", raw)
	}
}
