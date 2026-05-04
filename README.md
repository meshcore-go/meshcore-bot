# meshcore-bot

A configurable bot framework for [MeshCore](https://github.com/meshcore-dev/MeshCore) mesh networks, built with the pure Go [meshcore-go](https://github.com/meshcore-go/meshcore-go) library.

## Features

- **Trigger-based architecture**: Respond to group messages, private channel messages, or on a cron schedule.
- **Go template responses**: Access mesh data like sender, hops, path hashes, SNR, RSSI, and more.
- **Two node types**:
  - **KISS** (recommended): Direct radio control via hardware.
  - **Companion** (experimental): Piggyback on an existing MeshCore device via the companion client.
- **Private channel support**: Join private channels using a hex-encoded PSK.
- **MQTT integration**: Publish observed mesh traffic to MQTT brokers (e.g. [LetsMesh](https://letsmesh.net), [CoreScope](https://github.com/Kpa-clawbot/CoreScope)).
- **Hot-reload**: Reload configuration via `SIGHUP` without restarting. Reconnects the modem if connection settings change.
- **Multi-bot support**: Run multiple bots within a single instance.
- **Flexible configuration**: Supports TOML, YAML, and JSON formats.

## Installation

### Download a Release Binary (Recommended)

Pre-built binaries are available for Linux, macOS, and Windows on the [Releases](https://github.com/meshcore-go/meshcore-bot/releases) page.

1. Go to the [latest release](https://github.com/meshcore-go/meshcore-bot/releases/latest).
2. Download the binary for your platform (e.g. `meshcore-bot-linux-arm64` for a Raspberry Pi).
3. Make it executable and move it to a folder of your choosing:

```bash
chmod +x meshcore-bot-linux-arm64
sudo mv meshcore-bot-linux-arm64 /usr/local/bin/meshcore-bot
```

### Docker

Images are published to `ghcr.io/meshcore-go/meshcore-bot` for the following platforms:
`linux/386`, `linux/amd64`, `linux/arm/v6`, `linux/arm/v7`, `linux/arm64/v8`, `linux/ppc64le`, `linux/riscv64`, `linux/s390x`.

```bash
docker pull ghcr.io/meshcore-go/meshcore-bot:latest
```

### Build from Source

Requires Go 1.26.1+.

```bash
git clone https://github.com/meshcore-go/meshcore-bot.git
cd meshcore-bot
go build -o meshcore-bot
```

## Running

The bot looks for `config.toml`, `config.yaml`, `config.yml`, or `config.json` in the current directory.

### Step 1: Plug in your radio

Connect your MeshCore radio to your computer via USB. On Linux it usually shows up as `/dev/ttyACM0`. On macOS it's something like `/dev/cu.usbmodem*`.

On Linux, your user needs permission to access serial devices. Add yourself to the `dialout` group:

```bash
sudo usermod -a -G dialout $USER
```

Log out and back in (or reboot) for the change to take effect.

### Step 2: Create a config file

Create a file called `config.toml` in the same folder as the binary. Here's a minimal example that responds to "ping" on the `#testing` channel:

```toml
nodeType = "kiss"
connection = "serial:///dev/ttyACM0"

freq = 917.375
bw = 62.50
sf = 7
cr = 8
tx = 2

[[bot]]
name = "My Bot"

[[bot.trigger]]
type = "channel"
template = "Pong! Hello {{.Sender}}"
channels = ["#testing"]
match = ["(?i)^ping"]
```

### Step 3: Run it

```bash
./meshcore-bot
```

That's it. The bot will connect to your radio, join the `#testing` channel, and reply "Pong! Hello \<sender\>" whenever someone sends a message starting with "ping".

### Using Docker

Mount your config file and pass through the serial device:

```bash
docker run -d \
  --device /dev/ttyACM0 \
  -v ./config.toml:/data/config.toml \
  ghcr.io/meshcore-go/meshcore-bot
```

For TCP connections (e.g. companion mode via a serial-to-TCP bridge), no `--device` is needed:

```bash
docker run -d \
  -v ./config.toml:/data/config.toml \
  ghcr.io/meshcore-go/meshcore-bot
```

Pass CLI flags directly:

```bash
docker run -d \
  --device /dev/ttyACM0 \
  -v ./my-config.toml:/my-config.toml \
  ghcr.io/meshcore-go/meshcore-bot \
  meshcore-bot -c /my-config.toml -vvv
```

## Configuration Reference

### Connection

| Field | Description | Default |
|-------|-------------|---------|
| `nodeType` | `"kiss"` (direct radio) or `"companion"` (piggyback on existing device) | `"kiss"` |
| `connection` | `serial:///dev/ttyACM0` or `tcp://host:port` | `serial:///dev/ttyACM0` |
| `baudRate` | Serial baud rate | `115200` |
| `logLevel` | Log level: `debug`, `info`, `warn`, `error`, `trace` (overridden by `-v` flags) | `info` |

### Radio Settings (KISS only)

| Field | Description | Default |
|-------|-------------|---------|
| `freq` | Frequency in MHz | `917.375` |
| `bw` | Bandwidth in kHz | `62.50` |
| `sf` | Spreading Factor | `7` |
| `cr` | Coding Rate | `8` |
| `tx` | TX Power | `2` |

### Triggers

Each bot has a `name` and an array of `triggers`. Every trigger has a `type` and a `template`.

#### Channels

The `channels` field accepts either plain channel names (for public/hashtag channels) or objects with a `privateKey` for private channels:

```toml
# Public channels — just use the name
channels = ["#general", "#testing"]

# Private channels — use the [[bot.trigger.channels]] syntax
[[bot.trigger.channels]]
name = "Secret Ops"
privateKey = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"

# Mix of Private channels — use the [[bot.trigger.channels]] syntax
[[bot.trigger.channels]]
name = "Secret Ops"
privateKey = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"

[[bot.trigger.channels]]
name = "#general"

[[bot.trigger.channels]]
name = "#testing"
```

#### Channel Trigger (`type = "channel"`)

Fires when a message is received on a channel that matches one of the `match` patterns.

| Field | Description |
|-------|-------------|
| `channels` | Channels to listen on |
| `match` | Array of [Go regular expressions](https://pkg.go.dev/regexp/syntax) to match against incoming messages |
| `template` | Go text/template for the response |
| `retryTimeout` | Seconds to wait for a repeater echo before retrying | `5` |
| `maxRetries` | Maximum number of send retries | `3` |

#### Cron Trigger (`type = "cron"`)

Fires on a schedule.

| Field | Description |
|-------|-------------|
| `schedule` | Cron expression (e.g. `"*/5 * * * *"`) |
| `channels` | Channels to send the message to |
| `template` | Go text/template for the message |
| `retryTimeout` | Seconds to wait for a repeater echo before retrying | `5` |
| `maxRetries` | Maximum number of send retries | `3` |

After sending a message, the bot listens for the message to be repeated back by a repeater. If no echo is heard within `retryTimeout` seconds, the message is re-sent, up to `maxRetries` times. This applies to both channel and cron triggers.

### Template Variables

**Channel Trigger:**
- `{{.Sender}}` — Sender's node name
- `{{.Channel}}` — Channel name
- `{{.Message}}` — Original message text
- `{{.Match}}` — Map of named regex capture groups
- `{{.Timestamp}}` — Message timestamp
- `{{.SNR}}` — Signal-to-Noise Ratio
- `{{.RSSI}}` — Received Signal Strength Indicator
- `{{.Hops}}` — Number of hops
- `{{.PathHashes}}` — Raw path hashes
- `{{.PathHashSize}}` — Size of path hashes

**Cron Trigger:**
- `{{.Time}}` — Current time
- `{{.Schedule}}` — The cron schedule string

**Built-in Functions:**
- `formatPathBytes` — Formats raw path hashes into a readable string.

## Example Configs

### KISS Node with Private Channel

```toml
nodeType = "kiss"
connection = "serial:///dev/ttyACM0"
baudRate = 115200

freq = 917.375
bw = 62.50
sf = 7
cr = 8
tx = 22

[[bot]]
name = "Ping Bot"

[[bot.trigger]]
type = "channel"
template = "@[{{.Sender}}] 🦈={{.PathHashSize}} 🦘={{.Hops}} 🛣️={{.PathHashes | formatPathBytes}}"
match = ["(?i)^test", "(?i)^ping"]

[[bot.trigger.channels]]
name = "MyPrivateChannel"
privateKey = "7d78eab105a663ab3504d99a0e5b1891"
```

### Companion Node

```toml
nodeType = "companion"
connection = "tcp://127.0.0.1:8001"

[[bot]]
name = "Companion Bot"

[[bot.trigger]]
type = "channel"
template = "I am running via companion! Hello {{.Sender}}"
channels = ["#testing"]
match = ["(?i)^!hello"]
```

### Cron Trigger

```toml
[[bot.trigger]]
type = "cron"
schedule = "*/5 * * * *"
channels = ["#testing"]
template = "Periodic update: The time is {{.Time}}"
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `-c, --config PATH` | Path to the configuration file |
| `-V, --version` | Print version and exit |
| `-v, --verbose` | Enable verbose debug logging |

## MQTT Integration

meshcore-bot can publish observed mesh traffic to MQTT brokers. This is used by services like [LetsMesh](https://letsmesh.net) to aggregate mesh network data.

Each `[[observer]]` defines an MQTT observer node that forwards packets to one or more brokers. A unique identity key file is used for authentication.

```toml
[[observer]]
name = "AKL Bot"
iataCode = "AKL"
keyFile = "mqtt_identity.key"
statusInterval = 300

[observer.advert]
enabled = true
interval = 86400
lat = -36.8485
lon = 174.7633

[[observer.broker]]
name = "US West (LetsMesh v1)"
enabled = true
transport = "wss"
host = "mqtt-us-v1.letsmesh.net"
port = 443
topicPrefix = "meshcore"
retainStatus = false
tlsEnabled = true
authType = "token"
audience = "mqtt-us-v1.letsmesh.net"

[[observer.broker]]
name = "Europe (LetsMesh v1)"
enabled = true
transport = "wss"
host = "mqtt-eu-v1.letsmesh.net"
port = 443
topicPrefix = "meshcore"
retainStatus = false
tlsEnabled = true
authType = "token"
audience = "mqtt-eu-v1.letsmesh.net"

[[observer.broker]]
name = "CoreScope NZ"
enabled = true
transport = "wss"
host = "meshcore-mqtt-1.baird.io"
port = 443
topicPrefix = "meshcore"
retainStatus = false
tlsEnabled = true
authType = "token"
audience = "meshcore-mqtt-1.baird.io"
```

| Observer Field | Description |
|----------------|-------------|
| `name` | Display name for this observer |
| `iataCode` | Location identifier (e.g. airport code) |
| `keyFile` | Path to the identity key file (created automatically if missing) |
| `statusInterval` | Seconds between status publishes (default: 300) |

| Broker Field | Description |
|--------------|-------------|
| `name` | Display name for this broker |
| `enabled` | Enable/disable this broker |
| `dedup` | Enable per-broker packet deduplication (default: `false`) |
| `transport` | `"wss"` (WebSocket Secure) or `"tcp"` |
| `host` | Broker hostname |
| `port` | Broker port |
| `path` | WebSocket path (default: none) |
| `topicPrefix` | MQTT topic prefix |
| `disallowedPacketTypes` | Packet types to exclude (e.g. `["ack", "advert"]`) |
| `retainStatus` | Retain status messages on the broker |
| `tlsEnabled` | Enable TLS |
| `tlsInsecure` | Skip TLS certificate verification |
| `authType` | `"token"`, `"basic"`, or `"none"` |
| `username` | Username for basic auth |
| `password` | Password for basic auth |
| `audience` | Token audience (for token auth) |

| Advert Field | Description |
|--------------|-------------|
| `enabled` | Enable periodic advert broadcasting |
| `interval` | Seconds between adverts (default: `86400` / once per day) |
| `lat` | Latitude in decimal degrees (optional) |
| `lon` | Longitude in decimal degrees (optional) |

When enabled, the observer broadcasts a signed companion advert over the mesh on startup and then repeats at the configured interval. This allows the node to appear in the mesh network as a visible participant. If `lat` and `lon` are provided, the advert includes location data.

## Hot Reload

Send a `SIGHUP` signal to the process to reload the configuration without restarting:

```bash
kill -SIGHUP $(pgrep meshcore-bot)
```

If the connection settings or radio parameters change, the bot will automatically reconnect.

## License

See [LICENSE](LICENSE).
