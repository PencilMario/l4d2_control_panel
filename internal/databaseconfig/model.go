package databaseconfig

import (
	"errors"
	"fmt"
	"strings"
)

type Connection struct {
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"pass"`
	Port     string `json:"port,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
}

type Config struct {
	Revision      int64        `json:"revision"`
	DefaultDriver string       `json:"driver_default"`
	Connections   []Connection `json:"connections"`
}

func Defaults() Config {
	return Config{DefaultDriver: "mysql", Connections: []Connection{
		{Name: "default", Driver: "default", Host: "localhost", Database: "sourcemod", User: "root"},
		{Name: "storage-local", Driver: "sqlite", Database: "sourcemod-local"},
		{Name: "clientprefs", Driver: "sqlite", Host: "localhost", Database: "clientprefs-sqlite", User: "root"},
	}}
}

func Normalize(config Config) Config {
	config.DefaultDriver = strings.TrimSpace(config.DefaultDriver)
	for index := range config.Connections {
		connection := &config.Connections[index]
		connection.Name = strings.TrimSpace(connection.Name)
		connection.Driver = strings.TrimSpace(connection.Driver)
		connection.Host = strings.TrimSpace(connection.Host)
		connection.Database = strings.TrimSpace(connection.Database)
		connection.User = strings.TrimSpace(connection.User)
		connection.Port = strings.TrimSpace(connection.Port)
		connection.Timeout = strings.TrimSpace(connection.Timeout)
		if connection.Driver == "sqlite" && connection.Host == "localhost" {
			connection.Port = ""
			connection.Timeout = ""
		}
	}
	return config
}

func Validate(config Config) error {
	config = Normalize(config)
	if config.DefaultDriver == "" {
		return errors.New("default driver is required")
	}
	if len(config.Connections) == 0 {
		return errors.New("at least one database connection is required")
	}
	names := make(map[string]struct{}, len(config.Connections))
	for _, connection := range config.Connections {
		if connection.Name == "" || connection.Driver == "" || connection.Database == "" {
			return fmt.Errorf("connection name, driver, and database are required")
		}
		if _, exists := names[connection.Name]; exists {
			return fmt.Errorf("duplicate connection name %q", connection.Name)
		}
		names[connection.Name] = struct{}{}
	}
	return nil
}
