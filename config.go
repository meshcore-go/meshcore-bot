package main

import (
	"encoding/json"
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type ChannelRef struct {
	Name       string `json:"name" yaml:"name" toml:"name"`
	PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty" toml:"privateKey,omitempty"`
}

func (cr *ChannelRef) UnmarshalText(text []byte) error {
	cr.Name = string(text)
	return nil
}

type ChannelList []ChannelRef

func (cl *ChannelList) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("channels must be an array: %w", err)
	}

	result := make(ChannelList, 0, len(raw))
	for _, item := range raw {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			result = append(result, ChannelRef{Name: s})
			continue
		}
		var ref ChannelRef
		if err := json.Unmarshal(item, &ref); err != nil {
			return fmt.Errorf("channel entry must be a string or {name, privateKey} object: %w", err)
		}
		result = append(result, ref)
	}
	*cl = result
	return nil
}

func (cl *ChannelList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("channels must be a sequence")
	}

	result := make(ChannelList, 0, len(value.Content))
	for _, node := range value.Content {
		switch node.Kind {
		case yaml.ScalarNode:
			result = append(result, ChannelRef{Name: node.Value})
		case yaml.MappingNode:
			var ref ChannelRef
			if err := node.Decode(&ref); err != nil {
				return fmt.Errorf("channel entry decode error: %w", err)
			}
			result = append(result, ref)
		default:
			return fmt.Errorf("channel entry must be a string or mapping")
		}
	}
	*cl = result
	return nil
}

type TriggerConfig struct {
	Type     string `json:"type" yaml:"type" toml:"type"` // group, private, dm, cron, cap, etc
	Template string `json:"template" yaml:"template" toml:"template"`

	// Message Overflow behaviour
	CharLimitBehaviour *string `json:"charLimitBehaviour" yaml:"charLimitBehaviour" toml:"charLimitBehaviour"` // e.g. truncate or split

	// Messages/DMs
	Match    *[]string    `json:"match" yaml:"match" toml:"match"`          // Patterns to match against (supports wildcards/regex)
	Channels *ChannelList `json:"channels" yaml:"channels" toml:"channels"` // Channels to listen on (strings or {name, privateKey} objects)
	Contacts *[]string    `json:"contacts" yaml:"contacts" toml:"contact"`  // What Contacts to listen in for DMs

	// Retry Settings
	RetryTimeout *int64 `json:"retryTimeout" yaml:"retryTimeout" toml:"retryTimeout"` // Stored as seconds
	MaxRetries   *int   `json:"maxRetries" yaml:"maxRetries" toml:"maxRetries"`

	// Path Hash Size: 1-4 = fixed size, 0 = mirror incoming packet's hash size, nil = default (1)
	PathHashSize *uint8 `json:"pathHashSize,omitempty" yaml:"pathHashSize,omitempty" toml:"pathHashSize,omitempty"`

	// Cron Trigger
	Schedule string `json:"schedule,omitempty" yaml:"schedule,omitempty" toml:"schedule,omitempty"`
}

type BotConfig struct {
	Name *string `json:"name" yaml:"name" toml:"name"` // Name of the Node - Used in Channel Messages

	Triggers []TriggerConfig `json:"triggers" yaml:"triggers" toml:"trigger"`
}

type Config struct {
	// Logging
	LogLevel *string `json:"logLevel" yaml:"logLevel" toml:"logLevel"`

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
