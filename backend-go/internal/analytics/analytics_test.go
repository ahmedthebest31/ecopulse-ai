package analytics

import (
	"strings"
	"testing"
)

func rec(ts, facility, name, equip, flag, severity string, ratio, power, energy float64, peak bool) TelemetryRecord {
	return TelemetryRecord{
		Timestamp:       ts,
		FacilityID:      facility,
		FacilityName:    name,
		FacilityType:    "commercial",
		EquipmentID:     equip,
		PowerKW:         power,
		RatioToBaseline: ratio,
		EnergyKWh:       energy,
		IsPeakHour:      peak,
		AnomalyFlag:     flag,
		AnomalySeverity: severity,
		CarbonKG:        energy * 0.85,
	}
}

func sampleRecords() []TelemetryRecord {
	return []TelemetryRecord{
		rec("2026-08-03T10:00:00+03:00", "facility-001", "Cairo Tower", "HVAC-A", "forced_spike", "critical", 1.5, 100, 1.7, false),
		rec("2026-08-03T10:01:00+03:00", "facility-001", "Cairo Tower", "HVAC-A", "forced_spike", "critical", 1.6, 110, 1.8, false),
		rec("2026-08-03T10:02:00+03:00", "facility-001", "Cairo Tower", "", "none", "", 1.0, 60, 1.0, false),
		rec("2026-08-03T11:00:00+03:00", "facility-002", "Green Park", "Elevator-1", "forced_spike", "critical", 1.7, 80, 1.3, false),
		rec("2026-08-03T12:00:00+03:00", "facility-001", "Cairo Tower", "Chiller-1", "micro_surge", "predictive_maintenance", 1.08, 65, 1.1, false),
		rec("2026-08-03T12:01:00+03:00", "facility-001", "Cairo Tower", "Chiller-1", "micro_surge", "predictive_maintenance", 1.12, 68, 1.2, false),
		rec("2026-08-03T12:02:00+03:00", "facility-001", "Cairo Tower", "Chiller-1", "micro_surge", "predictive_maintenance", 1.1, 66, 1.15, false),
		rec("2026-08-03T18:00:00+03:00", "facility-001", "Cairo Tower", "", "none", "", 1.0, 120, 2.0, true),
		rec("2026-08-03T18:01:00+03:00", "facility-002", "Green Park", "", "none", "", 1.0, 90, 1.5, true),
	}
}

func TestAnalyzeDetectsSpikeRuns(t *testing.T) {
	result := Analyze(sampleRecords(), Config{SpikeThresholdPercent: 30})
	if len(result.CriticalSpikes) != 2 {
		t.Fatalf("expected 2 spikes, got %d", len(result.CriticalSpikes))
	}
	first := result.CriticalSpikes[0]
	if first.FacilityID != "facility-001" || first.DurationMinutes != 2 {
		t.Fatalf("spike 1 wrong: %+v", first)
	}
	if first.Start != "2026-08-03T10:00:00+03:00" || first.End != "2026-08-03T10:01:00+03:00" {
		t.Fatalf("spike 1 window wrong: %s -> %s", first.Start, first.End)
	}
	if first.Severity != "critical" {
		t.Fatalf("spike 1 severity wrong: %s", first.Severity)
	}
	second := result.CriticalSpikes[1]
	if second.FacilityID != "facility-002" || second.DurationMinutes != 1 {
		t.Fatalf("spike 2 wrong: %+v", second)
	}
}

func TestAnalyzeDetectsMaintenanceRuns(t *testing.T) {
	result := Analyze(sampleRecords(), Config{SpikeThresholdPercent: 30})
	if len(result.MaintenanceAlerts) != 1 {
		t.Fatalf("expected 1 maintenance alert, got %d", len(result.MaintenanceAlerts))
	}
	alert := result.MaintenanceAlerts[0]
	if alert.EquipmentID != "Chiller-1" || alert.DurationMinutes != 3 {
		t.Fatalf("maintenance alert wrong: %+v", alert)
	}
	if !strings.Contains(alert.Recommendation, "Chiller-1") {
		t.Fatalf("recommendation must reference equipment: %s", alert.Recommendation)
	}
}

func TestAnalyzePeakMetrics(t *testing.T) {
	result := Analyze(sampleRecords(), Config{SpikeThresholdPercent: 30})
	// Total energy: 1.7+1.8+1.0+1.3+1.1+1.2+1.15+2.0+1.5 = 12.75
	// Peak energy: 2.0+1.5 = 3.5
	if result.PeakMetrics.PeakHourRecords != 2 {
		t.Fatalf("expected 2 peak records, got %d", result.PeakMetrics.PeakHourRecords)
	}
	if result.PeakMetrics.MaxDemandKW != 120 {
		t.Fatalf("expected max demand 120, got %v", result.PeakMetrics.MaxDemandKW)
	}
	if result.TotalEnergyKWh != 12.75 {
		t.Fatalf("expected total energy 12.75, got %v", result.TotalEnergyKWh)
	}
	if result.PeakMetrics.ShareOfTotalEnergyPct != 27.451 {
		t.Fatalf("expected peak share ~27.45, got %v", result.PeakMetrics.ShareOfTotalEnergyPct)
	}
}

func TestAnalyzeCounts(t *testing.T) {
	result := Analyze(sampleRecords(), Config{SpikeThresholdPercent: 30})
	if result.TotalRecords != 9 {
		t.Fatalf("expected 9 records, got %d", result.TotalRecords)
	}
	if result.FacilityCount != 2 {
		t.Fatalf("expected 2 facilities, got %d", result.FacilityCount)
	}
}

func TestAnalyzeEmptyInput(t *testing.T) {
	result := Analyze(nil, Config{})
	if result.TotalRecords != 0 || result.CriticalSpikes != nil || result.MaintenanceAlerts != nil {
		t.Fatalf("empty input should produce empty result, got %+v", result)
	}
	if result.SpikeThresholdPct != 30 {
		t.Fatalf("default threshold should be 30, got %v", result.SpikeThresholdPct)
	}
}
