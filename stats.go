package main

import (
	"context"
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	"github.com/meshcore-go/meshcore-go/hardware"
)

type RadioInfo struct {
	FreqHz  uint32
	BwHz    uint32
	SF      uint8
	CR      uint8
	TxPower uint8
}

type DeviceStats struct {
	NoiseFloor int16
	BatteryMV  uint16
	UptimeSecs uint32
}

type StatsProvider interface {
	RadioConfig() RadioInfo
	Stats(ctx context.Context) DeviceStats
}

type kissStatsProvider struct {
	modem     *hardware.KissModem
	radio     RadioInfo
	startTime time.Time
	log       *slog.Logger

	mu         sync.Mutex
	noiseFloor int16
	batteryMV  uint16
}

func NewKissStatsProvider(modem *hardware.KissModem, radio RadioInfo) *kissStatsProvider {
	p := &kissStatsProvider{
		modem:     modem,
		radio:     radio,
		startTime: time.Now(),
		log:       slog.Default().With("component", "stats", "type", "kiss"),
	}

	modem.OnHwResponse(hardware.HwResp(hardware.HW_CMD_GET_NOISE_FLOOR), p.onNoiseFloor)
	modem.OnHwResponse(hardware.HwResp(hardware.HW_CMD_GET_BATTERY), p.onBattery)

	return p
}

func (p *kissStatsProvider) RadioConfig() RadioInfo {
	return p.radio
}

func (p *kissStatsProvider) Stats(ctx context.Context) DeviceStats {
	if err := p.modem.GetNoiseFloor(); err != nil {
		p.log.Error("get noise floor", "error", err)
	}
	if err := p.modem.GetBattery(); err != nil {
		p.log.Error("get battery", "error", err)
	}

	// Give the modem a moment to respond.
	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	ds := DeviceStats{
		NoiseFloor: p.noiseFloor,
		BatteryMV:  p.batteryMV,
		UptimeSecs: uint32(time.Since(p.startTime).Seconds()),
	}
	p.log.Log(ctx, LevelTrace, "stats polled",
		"noise_floor", ds.NoiseFloor, "battery_mv", ds.BatteryMV,
		"uptime_secs", ds.UptimeSecs)
	return ds
}

func (p *kissStatsProvider) onNoiseFloor(_ byte, data []byte) {
	if len(data) < 2 {
		return
	}
	p.mu.Lock()
	p.noiseFloor = int16(binary.LittleEndian.Uint16(data[:2]))
	p.mu.Unlock()
}

func (p *kissStatsProvider) onBattery(_ byte, data []byte) {
	if len(data) < 2 {
		return
	}
	p.mu.Lock()
	p.batteryMV = binary.LittleEndian.Uint16(data[:2])
	p.mu.Unlock()
}
