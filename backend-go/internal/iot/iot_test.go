package iot

import (
	"math"
	"testing"
	"time"
)

func TestTierForKWh(t *testing.T) {
	cases := []struct {
		kwh  float64
		want int
	}{
		{kwh: 0, want: 1},
		{kwh: 49.99, want: 1},
		{kwh: 50, want: 2},
		{kwh: 99.99, want: 2},
		{kwh: 100, want: 3},
		{kwh: 199.99, want: 3},
		{kwh: 200, want: 4},
		{kwh: 349.99, want: 4},
		{kwh: 350, want: 5},
		{kwh: 649.9, want: 5},
		{kwh: 650, want: 6},
		{kwh: 999.9, want: 6},
		{kwh: 1000, want: 7},
		{kwh: 12345, want: 7},
	}
	for _, tc := range cases {
		if got := TierForKWh(tc.kwh); got != tc.want {
			t.Errorf("TierForKWh(%v) = %d, want %d", tc.kwh, got, tc.want)
		}
	}
}

func TestAlertLevel(t *testing.T) {
	cases := []struct {
		tier   int
		isPeak bool
		want   string
	}{
		{tier: 1, isPeak: false, want: AlertNormal},
		{tier: 3, isPeak: true, want: AlertNormal},
		{tier: 4, isPeak: false, want: AlertNormal},
		{tier: 4, isPeak: true, want: AlertWarning},
		{tier: 5, isPeak: true, want: AlertWarning},
		{tier: 6, isPeak: false, want: AlertWarning},
		{tier: 6, isPeak: true, want: AlertCritical},
		{tier: 7, isPeak: false, want: AlertCritical},
		{tier: 7, isPeak: true, want: AlertCritical},
	}
	for _, tc := range cases {
		if got := AlertLevel(tc.tier, tc.isPeak); got != tc.want {
			t.Errorf("AlertLevel(%d, %v) = %q, want %q", tc.tier, tc.isPeak, got, tc.want)
		}
	}
}

func TestLoadShedRecommended(t *testing.T) {
	cases := []struct {
		tier   int
		isPeak bool
		alert  string
		want   bool
	}{
		{tier: 1, isPeak: false, alert: AlertNormal, want: false},
		{tier: 6, isPeak: false, alert: AlertWarning, want: false},
		{tier: 6, isPeak: true, alert: AlertCritical, want: true},
		{tier: 5, isPeak: true, alert: AlertWarning, want: false},
		{tier: 7, isPeak: false, alert: AlertCritical, want: true},
	}
	for _, tc := range cases {
		if got := LoadShedRecommended(tc.tier, tc.isPeak, tc.alert); got != tc.want {
			t.Errorf("LoadShedRecommended(%d, %v, %q) = %v, want %v", tc.tier, tc.isPeak, tc.alert, got, tc.want)
		}
	}
}

// fixedClock returns a Registry whose wall clock reads a fixed instant, so
// tests can advance lastSeen deterministically.
func fixedClock() (*Registry, *time.Time) {
	r := NewRegistry(Config{})
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	return r, &now
}

func TestSubmitAccumulatesTodayKWh(t *testing.T) {
	r, _ := fixedClock()
	mustSubmit(t, r, "esp32-factory-001", "2026-08-03T12:00:00Z", 3600, 380, 0)
	mustSubmit(t, r, "esp32-factory-001", "2026-08-03T12:00:05Z", 3600, 380, 0)

	devices := r.Devices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if abs(devices[0].TodayKWh-5.0) > 1e-6 {
		t.Fatalf("expected today_kwh=5.0 (3600 kW for 5s), got %v", devices[0].TodayKWh)
	}
	if devices[0].CurrentTier != 1 {
		t.Fatalf("expected tier 1 for 5 kWh, got %d", devices[0].CurrentTier)
	}
}

func TestSubmitResetsAtUTCDayChange(t *testing.T) {
	r, _ := fixedClock()
	mustSubmit(t, r, "d1", "2026-08-03T12:00:00Z", 3600, 380, 0)
	mustSubmit(t, r, "d1", "2026-08-03T12:00:05Z", 0, 380, 0)
	mustSubmit(t, r, "d1", "2026-08-04T00:00:00Z", 3600, 380, 0)

	devices := r.Devices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].TodayKWh != 0 {
		t.Fatalf("expected today_kwh reset to 0 on UTC day change, got %v", devices[0].TodayKWh)
	}
}

func TestSubmitOutOfOrderDoesNotAccumulate(t *testing.T) {
	r, _ := fixedClock()
	mustSubmit(t, r, "d1", "2026-08-03T12:00:00Z", 50, 380, 0)
	mustSubmit(t, r, "d1", "2026-08-03T11:59:55Z", 50, 380, 0)

	devices := r.Devices()
	if devices[0].TodayKWh != 0 {
		t.Fatalf("out-of-order reading must not add negative energy, got %v", devices[0].TodayKWh)
	}
}

func TestSubmitPeakHourAndResponse(t *testing.T) {
	r, _ := fixedClock()
	resp, err := r.Submit(Reading{
		DeviceID: "d1", Timestamp: "2026-08-03T18:30:00Z",
		Amperage: 15, Voltage: 380, Kilowatts: 9.9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if !resp.IsPeakHour {
		t.Fatal("18:30 must be inside the default 18:00-22:00 peak window")
	}
	if resp.CurrentTier != 1 {
		t.Fatalf("expected tier 1 on first reading, got %d", resp.CurrentTier)
	}
	if resp.AlertLevel != AlertNormal || resp.LoadShedRecommended {
		t.Fatalf("expected NORMAL, no shed on tier 1; got %q/%v", resp.AlertLevel, resp.LoadShedRecommended)
	}
}

func TestSetPeakWindowWrap(t *testing.T) {
	r, _ := fixedClock()
	r.SetPeakWindow("22:00", "06:00")
	late := mustSubmit(t, r, "d1", "2026-08-03T23:00:00Z", 1000, 380, 0)
	if !late.IsPeakHour {
		t.Fatal("23:00 must be inside the 22:00-06:00 wrapping window")
	}
	midday := mustSubmit(t, r, "d1", "2026-08-03T12:00:00Z", 1000, 380, 0)
	if midday.IsPeakHour {
		t.Fatal("12:00 must be outside the 22:00-06:00 window")
	}
	r.SetPeakWindow("garbage", "06:00")
	stillLate := mustSubmit(t, r, "d1", "2026-08-03T23:00:00Z", 1000, 380, 0)
	if !stillLate.IsPeakHour {
		t.Fatal("invalid window must be ignored, keeping the previous window")
	}
}

func TestSubmitEvictsOldest(t *testing.T) {
	r, now := fixedClock()
	r.maxDevices = 2
	mustSubmit(t, r, "d1", "2026-08-03T12:00:00Z", 1, 380, 0)
	*now = now.Add(2 * time.Second)
	mustSubmit(t, r, "d2", "2026-08-03T12:00:00Z", 1, 380, 0)
	*now = now.Add(2 * time.Second)
	mustSubmit(t, r, "d3", "2026-08-03T12:00:00Z", 1, 380, 0)

	devices := r.Devices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices after eviction, got %d", len(devices))
	}
	if devices[0].DeviceID == "d1" || devices[1].DeviceID == "d1" {
		t.Fatal("oldest device d1 must have been evicted")
	}
}

func TestSubmitValidationErrors(t *testing.T) {
	base := Reading{DeviceID: "d1", Timestamp: "2026-08-03T12:00:00Z", Kilowatts: 1}
	longID := make([]byte, 129)
	for i := range longID {
		longID[i] = 'a'
	}
	cases := []struct {
		name    string
		reading Reading
	}{
		{name: "empty device id", reading: func() Reading { r := base; r.DeviceID = " "; return r }()},
		{name: "oversized device id", reading: Reading{DeviceID: string(longID), Timestamp: base.Timestamp}},
		{name: "empty timestamp", reading: Reading{DeviceID: "d1", Timestamp: " "}},
		{name: "invalid timestamp", reading: Reading{DeviceID: "d1", Timestamp: "2026-08-03 12:00:00"}},
		{name: "negative kilowatts", reading: Reading{DeviceID: "d1", Timestamp: base.Timestamp, Kilowatts: -1}},
		{name: "nan kilowatts", reading: Reading{DeviceID: "d1", Timestamp: base.Timestamp, Kilowatts: math.NaN()}},
		{name: "infinite amperage", reading: Reading{DeviceID: "d1", Timestamp: base.Timestamp, Amperage: math.Inf(1)}},
		{name: "oversized voltage", reading: Reading{DeviceID: "d1", Timestamp: base.Timestamp, Voltage: 600}},
	}
	r, _ := fixedClock()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.Submit(tc.reading); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestSubmitRejectsRFC3339Violation(t *testing.T) {
	r, _ := fixedClock()
	if _, err := r.Submit(Reading{DeviceID: "d1", Timestamp: "not-a-time", Kilowatts: 1}); err == nil {
		t.Fatal("expected error for non-RFC3339 timestamp")
	}
}

func mustSubmit(t *testing.T, r *Registry, deviceID, timestamp string, kw, volts, amps float64) Response {
	t.Helper()
	resp, err := r.Submit(Reading{
		DeviceID: deviceID, Timestamp: timestamp,
		Amperage: amps, Voltage: volts, Kilowatts: kw,
	})
	if err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}
	return resp
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
