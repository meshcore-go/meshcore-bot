package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type packetMessage struct {
	Timestamp  string `json:"timestamp"`
	OriginID   string `json:"origin_id"`
	Origin     string `json:"origin"`
	Type       string `json:"type"`
	Direction  string `json:"direction"`
	Time       string `json:"time"`
	Date       string `json:"date"`
	Len        string `json:"len"`
	PacketType string `json:"packet_type"`
	Route      string `json:"route"`
	PayloadLen string `json:"payload_len"`
	Raw        string `json:"raw"`
	SNR        string `json:"SNR"`
	RSSI       string `json:"RSSI"`
	Hash       string `json:"hash"`
}

func formatPacket(pkt *meshcore.Packet, rawBytes []byte, originName, originID, direction string) ([]byte, error) {
	now := time.Now()

	route := "F"
	if pkt.IsRouteDirect() {
		route = "D"
	}

	hash := pkt.PacketHash()

	msg := packetMessage{
		Timestamp:  now.Format("2006-01-02T15:04:05.000000"),
		OriginID:   originID,
		Origin:     originName,
		Type:       "PACKET",
		Direction:  direction,
		Time:       now.Format("15:04:05"),
		Date:       fmt.Sprintf("%d/%d/%d", now.Day(), int(now.Month()), now.Year()),
		Len:        fmt.Sprintf("%d", len(rawBytes)),
		PacketType: fmt.Sprintf("%d", pkt.PayloadType()),
		Route:      route,
		PayloadLen: fmt.Sprintf("%d", len(pkt.Payload)),
		Raw:        strings.ToUpper(hex.EncodeToString(rawBytes)),
		SNR:        fmt.Sprintf("%d", pkt.SNR),
		RSSI:       fmt.Sprintf("%d", pkt.RSSI),
		Hash:       strings.ToUpper(hex.EncodeToString(hash[:])),
	}

	return json.Marshal(msg)
}

type statsBlock struct {
	UptimeSecs      uint32 `json:"uptime_secs"`
	PacketsReceived uint64 `json:"packets_received"`
	PacketsSent     uint64 `json:"packets_sent"`
	RecvErrors      uint64 `json:"recv_errors"`
	QueueLen        int    `json:"queue_len"`
}

type statusMessage struct {
	Status          string     `json:"status"`
	Timestamp       string     `json:"timestamp"`
	Origin          string     `json:"origin"`
	OriginID        string     `json:"origin_id"`
	Model           string     `json:"model"`
	FirmwareVersion string     `json:"firmware_version"`
	Radio           string     `json:"radio"`
	ClientVersion   string     `json:"client_version"`
	NoiseFloor      int16      `json:"noise_floor"`
	BatteryPercent  int        `json:"battery_percent"`
	Stats           statsBlock `json:"stats"`
}

func formatStatus(status, originName, originID string, radio RadioInfo, ds DeviceStats, packetsReceived, recvErrors uint64) ([]byte, error) {
	var radioStr string
	if radio.FreqHz > 0 {
		radioStr = fmt.Sprintf("%.3f,%.1f,%d,%d",
			float64(radio.FreqHz)/1_000_000,
			float64(radio.BwHz)/1_000,
			radio.SF,
			radio.CR,
		)
	}

	batteryPct := 100
	if ds.BatteryMV > 0 && ds.BatteryMV < 4200 {
		batteryPct = batteryPercentFromMV(ds.BatteryMV)
	}

	msg := statusMessage{
		Status:          status,
		Timestamp:       time.Now().Format("2006-01-02T15:04:05.000000"),
		Origin:          originName,
		OriginID:        originID,
		Model:           "meshcore-bot",
		FirmwareVersion: version,
		Radio:           radioStr,
		ClientVersion:   "meshcore-bot/" + version,
		NoiseFloor:      ds.NoiseFloor,
		BatteryPercent:  batteryPct,
		Stats: statsBlock{
			UptimeSecs:      ds.UptimeSecs,
			PacketsReceived: packetsReceived,
			RecvErrors:      recvErrors,
		},
	}
	return json.Marshal(msg)
}

func batteryPercentFromMV(mv uint16) int {
	switch {
	case mv >= 4200:
		return 100
	case mv <= 3200:
		return 0
	default:
		return int((float64(mv) - 3200) / (4200 - 3200) * 100)
	}
}
