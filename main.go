package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	companionClient "github.com/meshcore-go/meshcore-go/companion/client"
	companionTransport "github.com/meshcore-go/meshcore-go/companion/transport"
	"github.com/meshcore-go/meshcore-go/hardware"
	kissTransport "github.com/meshcore-go/meshcore-go/hardware/transport"
	"github.com/meshcore-go/meshcore-go/node"
	flag "github.com/spf13/pflag"
)

var version = "v1.0.1-dev"

const LevelTrace = slog.Level(-8)

var defaultConfigNames = []string{
	"config.toml",
	"config.yaml",
	"config.yml",
	"config.json",
}

type closerFunc func()

func (f closerFunc) Close() error { f(); return nil }

type modemState struct {
	modem       node.Modem
	companionCl *companionClient.Client
	stats       StatsProvider
	closers     []io.Closer
}

func (m *modemState) Close() {
	for i := len(m.closers) - 1; i >= 0; i-- {
		m.closers[i].Close()
	}
}

func main() {
	configPath := flag.StringP("config", "c", "", "path to config file (toml, yaml, or json)")
	showVersion := flag.BoolP("version", "V", false, "print version and exit")
	verbosity := flag.CountP("verbose", "v", "increase log verbosity (-v=debug, -vv=trace, -vvv=trace+)")
	flag.Parse()

	if *showVersion {
		fmt.Println("meshcore-bot", version)
		return
	}

	if *verbosity > 0 {
		level := slog.LevelDebug
		if *verbosity >= 2 {
			level = LevelTrace
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.LevelKey && a.Value.Any().(slog.Level) == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
				return a
			},
		})))
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Bots) == 0 && len(cfg.Observers) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no bots or observers configured")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	defer signal.Stop(sighup)

	ms, err := setupModem(ctx, cfg)
	if err != nil {
		slog.Error("modem setup failed", "error", err)
		os.Exit(1)
	}

	mux := node.NewRadioMux(ms.modem)

	bots, err := startBots(ctx, cfg, ms, mux)
	if err != nil {
		ms.Close()
		slog.Error("bot startup failed", "error", err)
		os.Exit(1)
	}

	observers, err := startObservers(ctx, cfg, mux, ms.modem, ms.stats)
	if err != nil {
		slog.Error("mqtt observer startup failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down...")
			stopObservers(observers)
			stopBots(bots)
			ms.Close()
			return

		case <-sighup:
			slog.Info("SIGHUP received, reloading config...")

			newCfg, err := loadConfig(*configPath)
			if err != nil {
				slog.Error("config reload failed, keeping current config", "error", err)
				continue
			}

			if len(newCfg.Bots) == 0 && len(newCfg.Observers) == 0 {
				slog.Error("reloaded config has no bots or observers, keeping current config")
				continue
			}

			stopObservers(observers)
			stopBots(bots)

			if modemConfigChanged(cfg, newCfg) {
				slog.Info("modem config changed, reconnecting...")
				ms.Close()

				ms, err = setupModem(ctx, newCfg)
				if err != nil {
					slog.Error("modem reconnect failed", "error", err)
					os.Exit(1)
				}
				mux = node.NewRadioMux(ms.modem)
			}

			bots, err = startBots(ctx, newCfg, ms, mux)
			if err != nil {
				slog.Error("bot restart failed after reload", "error", err)
				os.Exit(1)
			}

			observers, err = startObservers(ctx, newCfg, mux, ms.modem, ms.stats)
			if err != nil {
				slog.Error("mqtt observer restart failed after reload", "error", err)
			}

			cfg = newCfg
			slog.Info("config reloaded successfully")
		}
	}
}

func modemConfigChanged(old, new_ *Config) bool {
	if derefStr(old.NodeType) != derefStr(new_.NodeType) {
		return true
	}

	switch derefStr(old.NodeType) {
	case "companion":
		return derefStr(old.Connection) != derefStr(new_.Connection) ||
			derefInt(old.BaudRate) != derefInt(new_.BaudRate)
	case "kiss":
		return derefStr(old.Connection) != derefStr(new_.Connection) ||
			derefInt(old.BaudRate) != derefInt(new_.BaudRate) ||
			derefFloat(old.Freq) != derefFloat(new_.Freq) ||
			derefFloat(old.Bw) != derefFloat(new_.Bw) ||
			derefUint8(old.SF) != derefUint8(new_.SF) ||
			derefUint8(old.CR) != derefUint8(new_.CR) ||
			derefUint8(old.TX) != derefUint8(new_.TX)
	}

	return false
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefUint8(p *uint8) uint8 {
	if p == nil {
		return 0
	}
	return *p
}

func setupModem(ctx context.Context, cfg *Config) (*modemState, error) {
	ms := &modemState{}

	conn := *cfg.Connection
	connScheme, connAddr, ok := parseConnection(conn)
	if !ok {
		return nil, fmt.Errorf("invalid connection string: %s", conn)
	}

	switch *cfg.NodeType {
	case "kiss":
		var t hardware.Transport

		switch connScheme {
		case "serial":
			t = kissTransport.NewSerialTransport(kissTransport.SerialConfig{
				Port:     connAddr,
				BaudRate: *cfg.BaudRate,
			})
		case "tcp":
			t = kissTransport.NewTCPTransport(kissTransport.TCPConfig{
				Address: connAddr,
			})
		}

		kissModem := hardware.NewKissModem(t, hardware.WithSignalReport(true))

		connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
		defer connectCancel()

		if err := kissModem.Connect(connectCtx); err != nil {
			return nil, fmt.Errorf("kiss connect: %w", err)
		}
		ms.closers = append(ms.closers, kissModem)

		if err := kissModem.SetRadio(&hardware.RadioConfig{
			FreqHz: uint32(*cfg.Freq * 1000000),
			BwHz:   uint32(*cfg.Bw * 1000),
			SF:     *cfg.SF,
			CR:     *cfg.CR,
		}); err != nil {
			ms.Close()
			return nil, fmt.Errorf("SET_RADIO: %w", err)
		}
		slog.Info("SET_RADIO", "freq", *cfg.Freq, "bw", *cfg.Bw, "sf", *cfg.SF, "cr", *cfg.CR)

		if err := kissModem.SetTxPower(*cfg.TX); err != nil {
			ms.Close()
			return nil, fmt.Errorf("SET_TX_POWER: %w", err)
		}
		slog.Info("SET_TX_POWER", "tx", *cfg.TX)

		ms.stats = NewKissStatsProvider(kissModem, RadioInfo{
			FreqHz:  uint32(*cfg.Freq * 1000000),
			BwHz:    uint32(*cfg.Bw * 1000),
			SF:      *cfg.SF,
			CR:      *cfg.CR,
			TxPower: *cfg.TX,
		})

		ms.modem = kissModem

	case "companion":
		var t companionTransport.Transport

		switch connScheme {
		case "serial":
			t = companionTransport.NewSerialTransport(companionTransport.SerialConfig{
				Port:     connAddr,
				BaudRate: *cfg.BaudRate,
			})
		case "tcp":
			t = companionTransport.NewTCPTransport(companionTransport.TCPConfig{
				Address: connAddr,
			})
		}

		client := companionClient.New(t)
		ms.companionCl = client
		client.SetErrorHandler(func(err error) {
			slog.Error("companion error", "error", err)
		})

		connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
		defer connectCancel()

		if err := client.Connect(connectCtx); err != nil {
			return nil, fmt.Errorf("companion connect: %w", err)
		}
		ms.closers = append(ms.closers, client)

		selfInfo, err := client.AppStart(connectCtx, 1, "meshcore-bot")
		if err != nil {
			ms.Close()
			return nil, fmt.Errorf("companion app start: %w", err)
		}
		ms.stats = NewCompanionStatsProvider(client, selfInfo)

		compModem := companionClient.NewCompanionModem(ctx, client)
		ms.closers = append(ms.closers, closerFunc(compModem.Close))

		ms.modem = compModem

	default:
		return nil, fmt.Errorf("unsupported node type: %s", *cfg.NodeType)
	}

	return ms, nil
}

func startBots(ctx context.Context, cfg *Config, ms *modemState, mux *node.RadioMux) ([]*Bot, error) {

	var sf SenderFactory
	switch *cfg.NodeType {
	case "kiss":
		sf = func(n *node.Node) Sender { return NewNodeSender(n) }
	case "companion":
		cs, err := NewCompanionSender(ctx, ms.companionCl)
		if err != nil {
			return nil, fmt.Errorf("companion sender init: %w", err)
		}
		sf = func(_ *node.Node) Sender { return cs }
	}

	var bots []*Bot
	for _, botCfg := range cfg.Bots {
		b, err := NewBot(botCfg, mux, sf)
		if err != nil {
			stopBots(bots)
			return nil, fmt.Errorf("creating bot %q: %w", derefStr(botCfg.Name), err)
		}
		if err := b.Start(ctx); err != nil {
			stopBots(bots)
			return nil, fmt.Errorf("starting bot %q: %w", derefStr(botCfg.Name), err)
		}
		bots = append(bots, b)
		slog.Info("started bot", "bot", *botCfg.Name)
	}

	return bots, nil
}

func stopBots(bots []*Bot) {
	for _, b := range bots {
		b.Stop()
	}
}

func startObservers(ctx context.Context, cfg *Config, mux *node.RadioMux, modem node.Modem, stats StatsProvider) ([]*MqttObserver, error) {
	var observers []*MqttObserver
	for _, obsCfg := range cfg.Observers {
		keyFile := "mqtt_identity.key"
		if obsCfg.KeyFile != nil && *obsCfg.KeyFile != "" {
			keyFile = *obsCfg.KeyFile
		}

		id, err := loadOrCreateIdentity(keyFile)
		if err != nil {
			return nil, fmt.Errorf("mqtt identity: %w", err)
		}

		obs, err := NewMqttObserver(obsCfg, mux, modem, id, stats)
		if err != nil {
			stopObservers(observers)
			return nil, fmt.Errorf("creating mqtt observer: %w", err)
		}
		if err := obs.Start(ctx); err != nil {
			stopObservers(observers)
			return nil, fmt.Errorf("starting mqtt observer: %w", err)
		}
		observers = append(observers, obs)
		slog.Info("started mqtt observer", "name", derefStr(obsCfg.Name), "pubkey", publicKeyHex(id)[:16]+"...")
	}
	return observers, nil
}

func stopObservers(observers []*MqttObserver) {
	for _, o := range observers {
		o.Stop()
	}
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		return loadConfigFromCwd()
	}
	return loadConfigFromPath(path)
}

func loadConfigFromCwd() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	for _, name := range defaultConfigNames {
		p := filepath.Join(cwd, name)
		if _, err := os.Stat(p); err == nil {
			slog.Info("using config", "path", p)
			return loadConfigFromPath(p)
		}
	}

	return nil, fmt.Errorf("no config file found in %s (tried %s)", cwd, strings.Join(defaultConfigNames, ", "))
}

func loadConfigFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return UnmarshalConfingToml(data)
	case ".yaml", ".yml":
		return UnmarshalConfingYaml(data)
	case ".json":
		return UnmarshalConfingJson(data)
	default:
		return nil, fmt.Errorf("unsupported config format %q", ext)
	}
}

func parseConnection(conn string) (scheme, addr string, ok bool) {
	for _, prefix := range []string{"serial://", "tcp://"} {
		if strings.HasPrefix(conn, prefix) {
			return strings.TrimSuffix(prefix, "://"), conn[len(prefix):], true
		}
	}
	return "", "", false
}
