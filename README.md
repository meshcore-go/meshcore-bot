# meshcore-bot

meshcore-bot - A configurable bot framework for MeshCore mesh networks, built with the pure Go [meshcore-go](https://github.com/meshcore-go/meshcore-go) library.

## Features

- **Trigger-based architecture**:
  - **Group message triggers**: Listen to group channel messages with optional regex matching.
  - **Cron scheduled triggers**: Fire actions on a defined cron schedule.
- **Go template responses**: Access mesh data like sender, hops, path hashes, SNR, RSSI, and more in your responses.
- **Two node types**:
  - **KISS**: Direct radio control via hardware.
  - **Companion**: Piggyback on an existing MeshCore device via the companion client.
- **Hot-reload**: Reload configuration via `SIGHUP` without restarting the process. Reconnects the modem if connection settings change.
- **Multi-bot support**: Run multiple bots within a single instance.
- **Flexible configuration**: Supports TOML, YAML, and JSON formats.

## Quick Start

### Installation

Ensure you have Go 1.26+ installed.

```bash
go install github.com/meshcore-go/meshcore-bot@latest
```

Alternatively, clone and build:

```bash
git clone https://github.com/meshcore-go/meshcore-bot.git
cd meshcore-bot
go build -o meshcore-bot
```

### Running

The bot looks for `config.toml`, `config.yaml`, `config.yml`, or `config.json` in the current directory by default.

```bash
./meshcore-bot
```

## Configuration

The configuration file defines how the bot connects to the mesh and what triggers it responds to.

### Node Types

- `kiss`: Direct radio control. Requires radio settings.
- `companion`: Connects to an existing MeshCore device.

### Connection URI

Connection strings use the format `scheme://address`:
- `serial:///dev/ttyACM0`
- `tcp://127.0.0.1:8001`

### Radio Settings (KISS only)

- `freq`: Frequency in MHz (e.g., 917.375)
- `bw`: Bandwidth in kHz (e.g., 62.5)
- `sf`: Spreading Factor (e.g., 7)
- `cr`: Coding Rate (e.g., 8)
- `tx`: TX Power (e.g., 2)

### Bot Definition

Each bot has a `name` and an array of `triggers`.

#### Trigger Types

**Group Trigger (`type = "group"`)**
- `match`: Array of regex patterns to match against incoming messages.
- `channels`: List of channels to listen on (e.g., `["#testing"]`).
- `template`: Go text/template for the response.

**Cron Trigger (`type = "cron"`)**
- `schedule`: Cron expression (e.g., `0 * * * *`).
- `channels`: List of channels to send the message to.
- `template`: Go text/template for the message.

### Template System

Responses use Go's `text/template` engine.

**Available Variables (Group Trigger):**
- `{{.Sender}}`: Sender's node ID.
- `{{.Channel}}`: Channel name.
- `{{.Message}}`: Original message text.
- `{{.Match}}`: Map of named regex capture groups.
- `{{.Timestamp}}`: Message timestamp.
- `{{.SNR}}`: Signal-to-Noise Ratio.
- `{{.RSSI}}`: Received Signal Strength Indicator.
- `{{.Hops}}`: Number of hops.
- `{{.PathHashes}}`: Raw path hashes.
- `{{.PathHashSize}}`: Size of path hashes.

**Available Variables (Cron Trigger):**
- `{{.Time}}`: Current time.
- `{{.Schedule}}`: The cron schedule string.

**Built-in Functions:**
- `formatPathBytes`: Formats raw path hashes into a readable string.

## Example Configs

### KISS Node (Direct Radio)

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
type = "group"
template = "@[{{.Sender}}] 🦈={{.PathHashSize}} 🦘={{.Hops}} 🛣️={{.PathHashes | formatPathBytes}}"
channels = ["#testing"]
match = ["(?i)^test*", "(?i)^ping*"]
```

### Companion Node

```toml
nodeType = "companion"
connection = "tcp://127.0.0.1:8001"

[[bot]]
name = "Companion Bot"

[[bot.trigger]]
type = "group"
template = "I am running via companion! Hello {{.Sender}}"
channels = ["#westest"]
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

- `-c, --config PATH`: Path to the configuration file.
- `-V, --version`: Print version and exit.
- `-v, --verbose`: Enable verbose debug logging.

## Docker

Images are published to `ghcr.io/meshcore-go/meshcore-bot` for the following platforms:

- `linux/386`
- `linux/amd64`
- `linux/arm/v6`
- `linux/arm/v7`
- `linux/arm64/v8`
- `linux/ppc64le`
- `linux/riscv64`
- `linux/s390x`

```bash
docker pull ghcr.io/meshcore-go/meshcore-bot:latest
```

Mount your config file and pass through the serial device:

```bash
docker run --rm \
  --device /dev/ttyACM0 \
  -v ./config.toml:/config.toml \
  ghcr.io/meshcore-go/meshcore-bot
```

If the device path inside the container differs from the host, update `connection` in your config to match, or use a consistent path:

```bash
docker run --rm \
  --device /dev/ttyACM0:/dev/ttyACM0 \
  -v ./config.toml:/config.toml \
  ghcr.io/meshcore-go/meshcore-bot
```

For TCP connections (e.g. to a remote device or serial-to-TCP bridge), no `--device` is needed:

```bash
docker run --rm \
  -v ./config.toml:/config.toml \
  ghcr.io/meshcore-go/meshcore-bot
```

You can pass CLI flags directly:

```bash
docker run --rm \
  --device /dev/ttyACM0 \
  -v ./my-config.toml:/my-config.toml \
  ghcr.io/meshcore-go/meshcore-bot -c /my-config.toml -v
```

## Hot Reload

Send a `SIGHUP` signal to the process to reload the configuration:

```bash
kill -SIGHUP $(pgrep meshcore-bot)
```

If the connection settings or radio parameters change, the bot will automatically reconnect to the modem.

## License

MIT
