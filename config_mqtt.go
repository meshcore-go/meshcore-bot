package main

type MqttConfig struct {
	Name           *string        `json:"name" yaml:"name" toml:"name"`
	IataCode       *string        `json:"iataCode" yaml:"iataCode" toml:"iataCode"`
	KeyFile        *string        `json:"keyFile" yaml:"keyFile" toml:"keyFile"`
	StatusInterval *int           `json:"statusInterval" yaml:"statusInterval" toml:"statusInterval"`
	ObserveTX      *bool          `json:"observeTX" yaml:"observeTX" toml:"observeTX"`
	Owner          *string        `json:"owner" yaml:"owner" toml:"owner"`
	Email          *string        `json:"email" yaml:"email" toml:"email"`
	Brokers        []BrokerConfig `json:"brokers" yaml:"brokers" toml:"broker"`
}

type BrokerConfig struct {
	Name                  string   `json:"name" yaml:"name" toml:"name"`
	Enabled               bool     `json:"enabled" yaml:"enabled" toml:"enabled"`
	Transport             string   `json:"transport" yaml:"transport" toml:"transport"` // websockets or tcp
	Host                  string   `json:"host" yaml:"host" toml:"host"`
	Port                  int      `json:"port" yaml:"port" toml:"port"`
	TopicPrefix           string   `json:"topicPrefix" yaml:"topicPrefix" toml:"topicPrefix"` // e.g. "meshcore" for LetsMesh, or custom
	DisallowedPacketTypes []string `json:"disallowedPacketTypes" yaml:"disallowedPacketTypes" toml:"disallowedPacketTypes"`
	RetainStatus          bool     `json:"retainStatus" yaml:"retainStatus" toml:"retainStatus"`
	TlsEnabled            bool     `json:"tlsEnabled" yaml:"tlsEnabled" toml:"tlsEnabled"`
	TlsInsecure           bool     `json:"tlsInsecure" yaml:"tlsInsecure" toml:"tlsInsecure"`
	AuthType              string   `json:"authType" yaml:"authType" toml:"authType"` // token, basic, or none
	Username              string   `json:"username" yaml:"username" toml:"username"`
	Password              string   `json:"password" yaml:"password" toml:"password"`
	Path                  string   `json:"path" yaml:"path" toml:"path"` // WebSocket path (default: /mqtt)
	Audience              string   `json:"audience" yaml:"audience" toml:"audience"`
}

func (c *MqttConfig) statusIntervalSeconds() int {
	if c.StatusInterval != nil && *c.StatusInterval > 0 {
		return *c.StatusInterval
	}
	return 300
}
