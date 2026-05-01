package main

import (
	"encoding/json"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type TriggerConfig struct {
	Type     string `json:"type" yaml:"type" toml:"type"` // group, dm, cron, cap, etc
	Template string `json:"template" yaml:"template" toml:"template"`

	// Message Overflow behaviour
	CharLimitBehaviour *string `json:"charLimitBehaviour" yaml:"charLimitBehaviour" toml:"charLimitBehaviour"` // e.g. truncate or split

	// Messages/DMs
	Match    *[]string `json:"match" yaml:"match" toml:"match"`          // Patterns to match against (supports wildcards/regex)
	Channels *[]string `json:"channels" yaml:"channels" toml:"channels"` // What channels to listen in for Group Messages
	Contacts *[]string `json:"contacts" yaml:"contacts" toml:"contact"`  // What Contacts to listen in for DMs

	// Cron Trigger
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty" toml:"schedule,omitempty"`
}

type BotConfig struct {
	Name *string `json:"name" yaml:"name" toml:"name"` // Name of the Node - Used in Channel Messages

	Triggers []TriggerConfig `json:"triggers" yaml:"triggers" toml:"trigger"`
}

type Config struct {
	// Connection Settings
	NodeType   *string `json:"nodeType" yaml:"nodeType" toml:"nodeType"`       // kiss or companion
	Connection *string `json:"connection" yaml:"connection" toml:"connection"` // serial://<path> or tcp://<host:port>
	BaudRate   *int    `json:"baudRate" yaml:"baudRate" toml:"baudRate"`       // Default 115200 if using serial

	// Radio Settings - Only for KISS Radio
	Freq *float64 `json:"freq" yaml:"freq" toml:"freq"` // e.g. 917.375
	Bw   *float64 `json:"bw" yaml:"bw" toml:"bw"`       // e.g. 62.50
	SF   *uint8   `json:"sf" yaml:"sf" toml:"sf"`       // e.g. 7
	CR   *uint8   `json:"cr" yaml:"cr" toml:"cr"`       // e.g. 8
	TX   *uint8   `json:"tx" yaml:"tx" toml:"tx"`       // TX Power e.g. 22

	// Bots
	Bots []BotConfig `json:"bots" yaml:"bots" toml:"bot"`

	// MQTT Observers/Publishers
	Observers []MqttConfig `json:"observers" yaml:"observers" toml:"observer"`
}

func DefaultConfig() Config {
	nodeType := "kiss"
	connection := "serial:///dev/ttyACM0"
	baudRate := 115200
	freq := 917.375
	bw := 62.50
	sf := uint8(7)
	cr := uint8(8)
	tx := uint8(2)

	return Config{
		NodeType:   &nodeType,
		Connection: &connection,
		BaudRate:   &baudRate,
		Freq:       &freq,
		Bw:         &bw,
		SF:         &sf,
		CR:         &cr,
		TX:         &tx,
	}
}

func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if c.NodeType == nil {
		c.NodeType = defaults.NodeType
	}
	if c.Connection == nil {
		c.Connection = defaults.Connection
	}
	if c.BaudRate == nil {
		c.BaudRate = defaults.BaudRate
	}
	if c.Freq == nil {
		c.Freq = defaults.Freq
	}
	if c.Bw == nil {
		c.Bw = defaults.Bw
	}
	if c.SF == nil {
		c.SF = defaults.SF
	}
	if c.CR == nil {
		c.CR = defaults.CR
	}
	if c.TX == nil {
		c.TX = defaults.TX
	}
}

func UnmarshalConfigJson(data []byte) (*Config, error) {
	var cfg Config
	err := json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func UnmarshalConfigYaml(data []byte) (*Config, error) {
	var cfg Config
	err := yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func UnmarshalConfigToml(data []byte) (*Config, error) {
	var cfg Config
	err := toml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}
