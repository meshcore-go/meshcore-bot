package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	meshcore "github.com/meshcore-go/meshcore-go"
	"github.com/meshcore-go/meshcore-go/node"
)

type brokerClient struct {
	cfg      BrokerConfig
	client   mqtt.Client
	pubKeyHx string
	iata     string
	prefix   string

	disallowed map[byte]bool
	dedup      *meshcore.DedupCache // nil when dedup disabled for this broker
}

func (b *brokerClient) packetTopic() string {
	return fmt.Sprintf("%s/%s/%s/packets", b.prefix, b.iata, b.pubKeyHx)
}

func (b *brokerClient) statusTopic() string {
	return fmt.Sprintf("%s/%s/%s/status", b.prefix, b.iata, b.pubKeyHx)
}

func (b *brokerClient) isAllowed(payloadType byte) bool {
	return !b.disallowed[payloadType]
}

type MqttObserver struct {
	radio node.MuxRadio
	id    meshcore.LocalIdentity
	stats StatsProvider
	log   *slog.Logger

	cfg             MqttConfig
	originName      string
	pubKeyHx        string
	brokers         []*brokerClient
	packetsReceived atomic.Uint64
	floodRx         atomic.Uint64
	directRx        atomic.Uint64
	floodDups       atomic.Uint64
	directDups      atomic.Uint64
	recvErrors      *atomic.Uint64

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewMqttObserver(cfg MqttConfig, mux *node.RadioMux, id meshcore.LocalIdentity, stats StatsProvider, recvErrors *atomic.Uint64) (*MqttObserver, error) {
	name := "mqtt-observer"
	if cfg.Name != nil && *cfg.Name != "" {
		name = *cfg.Name
	}

	pkHex := publicKeyHex(id)
	radio := mux.NewRadio()

	obs := &MqttObserver{
		radio:      radio,
		id:         id,
		cfg:        cfg,
		stats:      stats,
		recvErrors: recvErrors,
		originName: name,
		pubKeyHx:   pkHex,
		log:        slog.Default().With("component", "mqtt", "observer", name),
	}

	return obs, nil
}

func (o *MqttObserver) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	o.cancel = cancel
	o.mu.Unlock()

	iata := "test"
	if o.cfg.IataCode != nil && *o.cfg.IataCode != "" {
		iata = *o.cfg.IataCode
	}

	for _, bcfg := range o.cfg.Brokers {
		if !bcfg.Enabled {
			continue
		}

		client, err := o.connectBroker(bcfg, iata)
		if err != nil {
			o.log.Error("broker connect failed", "broker", bcfg.Name, "error", err)
			continue
		}

		disallowed := parseDisallowed(bcfg.DisallowedPacketTypes)
		prefix := bcfg.TopicPrefix
		if prefix == "" {
			prefix = "meshcore"
		}

		bc := &brokerClient{
			cfg:        bcfg,
			client:     client,
			pubKeyHx:   o.pubKeyHx,
			iata:       iata,
			prefix:     prefix,
			disallowed: disallowed,
		}
		if bcfg.Dedup {
			bc.dedup = &meshcore.DedupCache{}
		}

		o.publishStatus(ctx, bc, "online")
		o.brokers = append(o.brokers, bc)
		o.log.Info("connected", "broker", bcfg.Name)
	}

	o.radio.SetPacketFilter(func(_ *meshcore.Packet) bool { return true })
	o.radio.SetRawDataHandler(o.onData)

	go o.heartbeatLoop(ctx)
	go o.tokenRefreshLoop(ctx)
	if o.cfg.Advert != nil && o.cfg.Advert.Enabled {
		go o.advertLoop(ctx)
	}

	return nil
}

func (o *MqttObserver) Stop() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, bc := range o.brokers {
		o.publishStatus(ctx, bc, "offline")
		bc.client.Disconnect(500)
	}
	o.brokers = nil
}

func (o *MqttObserver) onData(data []byte, snr int8, rssi int8) {
	o.log.Log(context.Background(), LevelTrace, "raw radio data",
		"len", len(data), "hex", strings.ToUpper(hex.EncodeToString(data)),
		"snr", snr, "rssi", rssi)

	pkt, err := meshcore.PacketFromBytes(data)
	if err != nil {
		o.log.Log(context.Background(), LevelTrace, "packet parse failed", "error", err)
		return
	}
	pkt.SNR = snr
	pkt.RSSI = rssi

	o.packetsReceived.Add(1)
	if pkt.IsRouteDirect() {
		o.directRx.Add(1)
	} else {
		o.floodRx.Add(1)
	}
	o.publishPacket(pkt, data, "rx")
}

func (o *MqttObserver) publishPacket(pkt *meshcore.Packet, rawBytes []byte, direction string) {
	o.log.Log(context.Background(), LevelTrace, "new packet accepted",
		"direction", direction, "type", pkt.PayloadType(),
		"payload_len", len(pkt.Payload))

	payload, err := formatPacket(pkt, rawBytes, o.originName, o.pubKeyHx, direction)
	if err != nil {
		o.log.Error("format error", "error", err)
		return
	}

	for _, bc := range o.brokers {
		if !bc.isAllowed(pkt.PayloadType()) {
			o.log.Log(context.Background(), LevelTrace, "packet type filtered",
				"broker", bc.cfg.Name, "type", pkt.PayloadType())
			continue
		}
		if bc.dedup != nil && bc.dedup.HasSeen(pkt) {
			o.log.Log(context.Background(), LevelTrace, "dedup hit, skipping",
				"broker", bc.cfg.Name, "type", pkt.PayloadType())
			if pkt.IsRouteDirect() {
				o.directDups.Add(1)
			} else {
				o.floodDups.Add(1)
			}
			continue
		}
		o.log.Log(context.Background(), LevelTrace, "publishing packet",
			"broker", bc.cfg.Name, "topic", bc.packetTopic(), "direction", direction)
		token := bc.client.Publish(bc.packetTopic(), 0, false, payload)
		token.Wait()
		if err := token.Error(); err != nil {
			o.log.Error("publish error", "broker", bc.cfg.Name, "error", err)
		}
	}
}

func (o *MqttObserver) advertLoop(ctx context.Context) {
	// Send initial advert
	err := o.advert()
	if err != nil {
		o.log.Error("initial advert error", "error", err)
	}

	advertInterval := o.cfg.Advert.Interval
	if advertInterval == nil || *advertInterval < 1 {
		oneDay := 86400
		advertInterval = &oneDay
	}

	// Get tick
	interval := time.Duration(*advertInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Start loop
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := o.advert()
			if err != nil {
				o.log.Error("advert error", "error", err)
			}
		}
	}
}

func (o *MqttObserver) advert() error {
	appData := meshcore.AdvertAppData{
		Type: "CHAT",
		Name: *o.cfg.Name,
		Lat:  0,
		Lon:  0,
	}

	if o.cfg.Advert.hasLatLon() {
		appData.Lat = int32(math.Round(*o.cfg.Advert.Lat * 1_000_000.0))
		appData.Lon = int32(math.Round(*o.cfg.Advert.Lon * 1_000_000.0))
	}

	rawAppData, err := appData.ToBytes()
	if err != nil {
		return err
	}

	advert := meshcore.Advert{
		PublicKey:  o.id.Identity,
		Timestamp:  uint32(time.Now().Unix()),
		RawAppData: rawAppData,
	}
	advert.Sign(o.id.PrivateKey())

	payload, err := advert.ToBytes()
	if err != nil {
		return err
	}

	pkt := meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		PathLength: meshcore.PathHashSize - 1,
		Payload:    payload,
	}

	data, err := pkt.ToBytes()
	if err != nil {
		return err
	}

	return o.radio.SendData(data)
}

func (o *MqttObserver) heartbeatLoop(ctx context.Context) {
	interval := time.Duration(o.cfg.statusIntervalSeconds()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, bc := range o.brokers {
				o.publishStatus(ctx, bc, "online")
			}
		}
	}
}

func (o *MqttObserver) tokenRefreshLoop(ctx context.Context) {
	refreshAt := time.Duration(float64(tokenLifetime) * 0.8)
	ticker := time.NewTicker(refreshAt)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, bc := range o.brokers {
				if !strings.EqualFold(bc.cfg.AuthType, "token") {
					continue
				}
				o.log.Debug("refreshing token", "broker", bc.cfg.Name)
				bc.client.Disconnect(250)

				newClient, err := o.connectBroker(bc.cfg, bc.iata)
				if err != nil {
					o.log.Error("token refresh reconnect failed", "broker", bc.cfg.Name, "error", err)
					continue
				}
				bc.client = newClient
				o.publishStatus(ctx, bc, "online")
				o.log.Info("token refreshed", "broker", bc.cfg.Name)
			}
		}
	}
}

func (o *MqttObserver) publishStatus(ctx context.Context, bc *brokerClient, status string) {
	var radio RadioInfo
	var ds DeviceStats
	if o.stats != nil {
		radio = o.stats.RadioConfig()
		ds = o.stats.Stats(ctx)
	}

	packets := PacketCounts{
		Received:   o.packetsReceived.Load(),
		FloodRx:    o.floodRx.Load(),
		DirectRx:   o.directRx.Load(),
		FloodDups:  o.floodDups.Load(),
		DirectDups: o.directDups.Load(),
	}

	payload, err := formatStatus(status, o.originName, o.pubKeyHx, radio, ds, packets, o.recvErrors.Load())
	if err != nil {
		o.log.Error("status format error", "error", err)
		return
	}

	o.log.Log(ctx, LevelTrace, "publishing status",
		"broker", bc.cfg.Name, "topic", bc.statusTopic(),
		"json", string(payload))
	token := bc.client.Publish(bc.statusTopic(), 1, bc.cfg.RetainStatus, payload)
	token.Wait()
}

func (o *MqttObserver) connectBroker(bcfg BrokerConfig, iata string) (mqtt.Client, error) {
	var scheme string
	switch strings.ToLower(bcfg.Transport) {
	case "websockets", "ws", "wss":
		if bcfg.TlsEnabled {
			scheme = "wss"
		} else {
			scheme = "ws"
		}
	default:
		if bcfg.TlsEnabled {
			scheme = "tls"
		} else {
			scheme = "tcp"
		}
	}

	brokerURL := fmt.Sprintf("%s://%s:%d%s", scheme, bcfg.Host, bcfg.Port, bcfg.Path)
	clientID := fmt.Sprintf("meshcore_%s_%s", o.pubKeyHx[:16], bcfg.Host)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(clientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Minute)

	if bcfg.TlsEnabled {
		opts.SetTLSConfig(&tls.Config{
			InsecureSkipVerify: bcfg.TlsInsecure,
			MinVersion:         tls.VersionTLS12,
		})
	}

	switch strings.ToLower(bcfg.AuthType) {
	case "token":
		audience := bcfg.Audience
		if audience == "" {
			audience = bcfg.Host
		}
		token, _, err := generateToken(o.id, audience, derefStr(o.cfg.Email), derefStr(o.cfg.Owner))
		if err != nil {
			return nil, fmt.Errorf("generating auth token: %w", err)
		}
		opts.SetUsername(tokenUsername(o.id))
		opts.SetPassword(token)
	case "basic":
		opts.SetUsername(bcfg.Username)
		opts.SetPassword(bcfg.Password)
	}

	prefix := bcfg.TopicPrefix
	if prefix == "" {
		prefix = "meshcore"
	}
	statusTopic := fmt.Sprintf("%s/%s/%s/status", prefix, iata, o.pubKeyHx)

	// LWT uses minimal status (no live stats — we're about to disconnect).
	offlinePayload, _ := formatStatus("offline", o.originName, o.pubKeyHx, RadioInfo{}, DeviceStats{}, PacketCounts{}, 0)
	opts.SetWill(statusTopic, string(offlinePayload), 1, bcfg.RetainStatus)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", brokerURL, err)
	}

	return client, nil
}

var payloadTypeNames = map[string]byte{
	"req":        meshcore.PayloadTypeReq,
	"response":   meshcore.PayloadTypeResponse,
	"txt_msg":    meshcore.PayloadTypeTxtMsg,
	"ack":        meshcore.PayloadTypeAck,
	"advert":     meshcore.PayloadTypeAdvert,
	"grp_txt":    meshcore.PayloadTypeGrpTxt,
	"grp_data":   meshcore.PayloadTypeGrpData,
	"anon_req":   meshcore.PayloadTypeAnonReq,
	"path":       meshcore.PayloadTypePath,
	"trace":      meshcore.PayloadTypeTrace,
	"multi_part": meshcore.PayloadTypeMultiPart,
	"control":    meshcore.PayloadTypeControl,
	"raw_custom": meshcore.PayloadTypeRawCustom,
}

func parseDisallowed(names []string) map[byte]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[byte]bool, len(names))
	for _, name := range names {
		if v, ok := payloadTypeNames[strings.ToLower(name)]; ok {
			m[v] = true
		}
	}
	return m
}
