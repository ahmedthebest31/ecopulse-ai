// Package iot implements the device-telemetry ingestion side of the EcoPulse
// AI hardware simulator. It receives instantaneous readings from ESP32
// devices (the simulated and physical factory hardware), validates them,
// rolls per-device consumption per UTC day, and replies with the tariff tier
// and load-shed decision that the firmware actuates in the field.
package iot

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"backend-go/internal/tariff"
)

// Reading is the JSON body POSTed by a device to /api/telemetry/iot.
type Reading struct {
	DeviceID       string  `json:"device_id"`
	Timestamp      string  `json:"timestamp"`
	Amperage       float64 `json:"amperage"`
	Voltage        float64 `json:"voltage"`
	Kilowatts      float64 `json:"kilowatts"`
	AccumulatedKWh float64 `json:"accumulated_kwh"`
}

// Response is the actuation contract sent back to the device after each post.
// The ESP32 firmware parses these fields with plain string searches, so field
// names and the alert_level value must stay exactly as documented here.
type Response struct {
	Status              string `json:"status"`
	CurrentTier         int    `json:"current_tier"`
	IsPeakHour          bool   `json:"is_peak_hour"`
	LoadShedRecommended bool   `json:"load_shed_recommended"`
	AlertLevel          string `json:"alert_level"`
}

const (
	// AlertNormal keeps the device in normal operation.
	AlertNormal = "NORMAL"
	// AlertWarning raises operator awareness without shedding the load.
	AlertWarning = "WARNING"
	// AlertCritical sheds the connected load through the relay.
	AlertCritical = "CRITICAL"
)

const (
	defaultPeakStart  = "18:00"
	defaultPeakEnd    = "22:00"
	defaultMaxDevices = 4096

	maxAmperage    = 1000.0
	maxVoltage     = 500.0
	maxKilowatts   = 5000.0
	maxAccumulated = 1e9
	maxDeviceIDLen = 128
	maxDeltaSec    = 86400.0
)

// device holds the rolling consumption state of one ESP32.
type device struct {
	id       string
	lastSeen time.Time
	dayKey   string
	todayKWh float64
	lastTs   time.Time
	hasLast  bool
}

// Registry keeps the per-device rolling consumption state. All state is
// guarded by one mutex; device counts are bounded by Config.MaxDevices so a
// flood of spoofed device ids cannot grow memory without limit.
type Registry struct {
	mu         sync.RWMutex
	peakStart  int
	peakEnd    int
	wraps      bool
	maxDevices int
	devices    map[string]*device
	now        func() time.Time
}

// Config controls the registry's peak-hour window and device cap.
type Config struct {
	PeakStart  string
	PeakEnd    string
	MaxDevices int
}

// NewRegistry creates a registry with the given configuration. Empty peak
// bounds default to the 18:00-22:00 Egyptian peak window; a non-positive
// device cap defaults to defaultMaxDevices.
func NewRegistry(cfg Config) *Registry {
	if cfg.PeakStart == "" {
		cfg.PeakStart = defaultPeakStart
	}
	if cfg.PeakEnd == "" {
		cfg.PeakEnd = defaultPeakEnd
	}
	if cfg.MaxDevices <= 0 {
		cfg.MaxDevices = defaultMaxDevices
	}
	r := &Registry{
		maxDevices: cfg.MaxDevices,
		devices:    make(map[string]*device, 64),
		now:        time.Now,
	}
	r.setPeakWindow(cfg.PeakStart, cfg.PeakEnd)
	return r
}

// SetPeakWindow updates the peak-hour window after load. The bounds follow
// the analytics package: "HH:MM" clock values; start after end wraps across
// midnight. Invalid bounds are ignored so a corrupt dataset cannot silently
// switch the window.
func (r *Registry) SetPeakWindow(start, end string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setPeakWindow(start, end)
}

func (r *Registry) setPeakWindow(start, end string) {
	startMin, err := parseClock(start)
	if err != nil {
		return
	}
	endMin, err := parseClock(end)
	if err != nil {
		return
	}
	r.peakStart = startMin
	r.peakEnd = endMin
	r.wraps = startMin > endMin
}

// parseClock and inWindow replicate the analytics package's peak-window
// semantics (which are not exported) so the firmware decision surface and the
// dataset analytics always agree on what "peak hour" means.
func parseClock(value string) (int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("peak window %q is invalid; use HH:MM", value)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func inWindow(minutes, start, end int, wraps bool) bool {
	if wraps {
		return minutes >= start || minutes < end
	}
	return minutes >= start && minutes < end
}

func (r *Registry) isPeakHour(ts time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return inWindow(ts.Hour()*60+ts.Minute(), r.peakStart, r.peakEnd, r.wraps)
}

// Submit validates one device reading, folds it into the device's rolling
// daily consumption, and returns the actuation decision for the firmware.
func (r *Registry) Submit(reading Reading) (Response, error) {
	if err := validateReading(reading); err != nil {
		return Response{}, err
	}
	ts, err := time.Parse(time.RFC3339, reading.Timestamp)
	if err != nil {
		return Response{}, errors.New("timestamp must be an RFC3339 timestamp, e.g. 2026-08-03T18:00:00Z")
	}

	r.mu.Lock()
	d, ok := r.devices[reading.DeviceID]
	if !ok {
		if len(r.devices) >= r.maxDevices {
			r.evictOldest()
		}
		d = &device{id: reading.DeviceID}
		r.devices[reading.DeviceID] = d
	}

	dayKey := ts.UTC().Format("2006-01-02")
	if d.dayKey != dayKey {
		d.dayKey = dayKey
		d.todayKWh = 0
		d.hasLast = false
	}
	if d.hasLast {
		deltaSec := ts.Sub(d.lastTs).Seconds()
		// A timestamp far in the past or after a long offline gap is not
		// proportional to real load, so the delta is ignored rather than
		// distorting the day's total.
		if deltaSec > 0 && deltaSec <= maxDeltaSec {
			d.todayKWh += reading.Kilowatts * deltaSec / 3600.0
		}
	}
	d.lastTs = ts
	d.hasLast = true
	d.lastSeen = r.now()
	r.mu.Unlock()

	isPeak := r.isPeakHour(ts)
	tier := TierForKWh(d.todayKWh)
	alert := AlertLevel(tier, isPeak)
	return Response{
		Status:              "ok",
		CurrentTier:         tier,
		IsPeakHour:          isPeak,
		LoadShedRecommended: LoadShedRecommended(tier, isPeak, alert),
		AlertLevel:          alert,
	}, nil
}

// evictOldest drops the least-recently-seen device when the registry is at
// capacity. The scan is linear under the write lock, which is acceptable for
// bounded device counts on the modest 5-second reporting cadence.
func (r *Registry) evictOldest() {
	var victim *device
	for _, d := range r.devices {
		if victim == nil || d.lastSeen.Before(victim.lastSeen) {
			victim = d
		}
	}
	if victim != nil {
		delete(r.devices, victim.id)
	}
}

func validateReading(reading Reading) error {
	if strings.TrimSpace(reading.DeviceID) == "" {
		return errors.New("device_id must not be empty")
	}
	if len(reading.DeviceID) > maxDeviceIDLen {
		return fmt.Errorf("device_id must not exceed %d characters", maxDeviceIDLen)
	}
	if strings.TrimSpace(reading.Timestamp) == "" {
		return errors.New("timestamp must not be empty")
	}
	checks := []struct {
		value float64
		name  string
		max   float64
	}{
		{reading.Amperage, "amperage", maxAmperage},
		{reading.Voltage, "voltage", maxVoltage},
		{reading.Kilowatts, "kilowatts", maxKilowatts},
		{reading.AccumulatedKWh, "accumulated_kwh", maxAccumulated},
	}
	for _, c := range checks {
		if err := ensureBounded(c.value, c.name, c.max); err != nil {
			return err
		}
	}
	return nil
}

func ensureBounded(value float64, name string, max float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", name)
	}
	if value < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}
	if value > max {
		return fmt.Errorf("%s must not exceed %g", name, max)
	}
	return nil
}

// TierForKWh maps today's cumulative consumption to the 1-based Egyptian
// tariff tier using the same bracket boundaries as the tariff package.
func TierForKWh(kwh float64) int {
	tiers := tariff.DefaultEgyptianTiers()
	for i, tier := range tiers {
		if kwh < tier.LowerKWh {
			break
		}
		if tier.UpperKWh <= 0 || kwh < tier.UpperKWh {
			return i + 1
		}
	}
	return len(tiers)
}

// AlertLevel applies the load-shed policy to a device's tariff tier and peak
// membership. It is the single source of truth for the firmware actuation.
func AlertLevel(tier int, isPeak bool) string {
	if tier >= 7 || (isPeak && tier >= 6) {
		return AlertCritical
	}
	if (isPeak && tier >= 4) || tier >= 6 {
		return AlertWarning
	}
	return AlertNormal
}

// LoadShedRecommended mirrors the shed decision implied by AlertLevel: a
// critical state, or peak-time consumption at or above the 6th tariff tier.
func LoadShedRecommended(tier int, isPeak bool, alert string) bool {
	if alert == AlertCritical {
		return true
	}
	return isPeak && tier >= 6
}

// DeviceInfo is one live entry of the registry for the devices listing.
type DeviceInfo struct {
	DeviceID    string    `json:"device_id"`
	LastSeen    time.Time `json:"last_seen"`
	TodayKWh    float64   `json:"today_kwh"`
	CurrentTier int       `json:"current_tier"`
	AlertLevel  string    `json:"alert_level"`
}

// Devices returns a stable, sorted snapshot of the live registry.
func (r *Registry) Devices() []DeviceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	out := make([]DeviceInfo, 0, len(r.devices))
	for _, d := range r.devices {
		tier := TierForKWh(d.todayKWh)
		isPeak := inWindow(now.Hour()*60+now.Minute(), r.peakStart, r.peakEnd, r.wraps)
		out = append(out, DeviceInfo{
			DeviceID:    d.id,
			LastSeen:    d.lastSeen,
			TodayKWh:    math.Round(d.todayKWh*10000) / 10000,
			CurrentTier: tier,
			AlertLevel:  AlertLevel(tier, isPeak),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].DeviceID < out[j].DeviceID
	})
	return out
}
