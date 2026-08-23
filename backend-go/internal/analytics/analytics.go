// Package analytics aggregates telemetry records into executive metrics:
// critical consumption spikes, predictive maintenance alerts, and peak-hour
// load statistics. It operates on the records produced by the EcoPulse AI
// data generator.
package analytics

import (
	"fmt"
	"math"
	"time"
)

// TelemetryRecord mirrors one record of the generated telemetry dataset.
type TelemetryRecord struct {
	Timestamp       string  `json:"timestamp"`
	FacilityID      string  `json:"facility_id"`
	FacilityName    string  `json:"facility_name"`
	FacilityType    string  `json:"facility_type"`
	EquipmentID     string  `json:"equipment_id"`
	PowerKW         float64 `json:"power_kw"`
	BaselineKW      float64 `json:"baseline_kw"`
	RatioToBaseline float64 `json:"ratio_to_baseline"`
	EnergyKWh       float64 `json:"energy_kwh"`
	VoltageV        float64 `json:"voltage_v"`
	CurrentA        float64 `json:"current_a"`
	PowerFactor     float64 `json:"power_factor"`
	FrequencyHz     float64 `json:"frequency_hz"`
	IsPeakHour      bool    `json:"is_peak_hour"`
	AnomalyFlag     string  `json:"anomaly_flag"`
	AnomalySeverity string  `json:"anomaly_severity"`
	CarbonKG        float64 `json:"carbon_kg"`
}

// Config controls spike detection thresholds and the peak-hour window.
type Config struct {
	// SpikeThresholdPercent is consumption above baseline (ratio >= 1+p/100)
	// that marks a critical consumption spike. Defaults to 30.
	SpikeThresholdPercent float64

	// PeakStart/PeakEnd optionally override peak-hour membership as
	// "HH:MM" clock times derived from each record's timestamp instead of
	// trusting the dataset's is_peak_hour flag. Empty values keep the
	// flag-based default behavior. A window where Start > End wraps across
	// midnight.
	PeakStart string
	PeakEnd   string
}

// Spike describes a contiguous critical consumption anomaly for one facility.
type Spike struct {
	FacilityID      string  `json:"facility_id"`
	FacilityName    string  `json:"facility_name"`
	EquipmentID     string  `json:"equipment_id"`
	Start           string  `json:"start"`
	End             string  `json:"end"`
	DurationMinutes int     `json:"duration_minutes"`
	MaxPowerKW      float64 `json:"max_power_kw"`
	MaxRatio        float64 `json:"max_ratio_to_baseline"`
	Severity        string  `json:"severity"`
}

// MaintenanceAlert describes a gradual micro-surge window tagged for
// predictive maintenance on a specific piece of equipment.
type MaintenanceAlert struct {
	FacilityID      string  `json:"facility_id"`
	FacilityName    string  `json:"facility_name"`
	EquipmentID     string  `json:"equipment_id"`
	Start           string  `json:"start"`
	End             string  `json:"end"`
	DurationMinutes int     `json:"duration_minutes"`
	MaxRatio        float64 `json:"max_ratio_to_baseline"`
	Severity        string  `json:"severity"`
	Recommendation  string  `json:"recommendation"`
}

// PeakMetrics aggregates consumption during the configured peak window.
type PeakMetrics struct {
	PeakWindow            string  `json:"peak_window"`
	TotalEnergyKWh        float64 `json:"total_energy_kwh"`
	PeakHourRecords       int     `json:"peak_hour_records"`
	MaxDemandKW           float64 `json:"max_demand_kw"`
	ShareOfTotalEnergyPct float64 `json:"share_of_total_energy_pct"`
}

// Result is the full output of the analytics aggregation.
type Result struct {
	TotalRecords      int                `json:"total_records"`
	FacilityCount     int                `json:"facility_count"`
	SpikeThresholdPct float64            `json:"spike_threshold_percent"`
	CriticalSpikes    []Spike            `json:"critical_spikes"`
	MaintenanceAlerts []MaintenanceAlert `json:"maintenance_alerts"`
	PeakMetrics       PeakMetrics        `json:"peak_metrics"`
	TotalEnergyKWh    float64            `json:"total_energy_kwh"`
	TotalCarbonKG     float64            `json:"total_carbon_kg"`
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

// Analyze computes spike, maintenance, and peak-hour metrics from the records.
// Records are expected to be chronological; runs are detected by consecutive
// records with the same facility (and equipment, for maintenance).
//
// Critical spikes are detected by magnitude: any record whose consumption is
// at least SpikeThresholdPercent above its baseline starts or continues a
// spike run, independent of the generator's anomaly_flag. Predictive
// maintenance remains flag-based on micro_surge markers.
func Analyze(records []TelemetryRecord, cfg Config) Result {
	if cfg.SpikeThresholdPercent <= 0 {
		cfg.SpikeThresholdPercent = 30
	}
	minSpikeRatio := 1 + cfg.SpikeThresholdPercent/100

	// Peak membership: custom window derived from timestamps when both clock
	// bounds are provided, otherwise the dataset flag as before.
	useCustomPeak := cfg.PeakStart != "" && cfg.PeakEnd != ""
	var peakStartMin, peakEndMin int
	wraps := false
	if useCustomPeak {
		start, err1 := parseClock(cfg.PeakStart)
		end, err2 := parseClock(cfg.PeakEnd)
		if err1 == nil && err2 == nil {
			peakStartMin = start
			peakEndMin = end
			wraps = start > end
		} else {
			useCustomPeak = false
		}
	}

	result := Result{
		TotalRecords:      len(records),
		SpikeThresholdPct: cfg.SpikeThresholdPercent,
	}
	facilities := make(map[string]struct{})
	var peakEnergy, totalEnergy, totalCarbon, peakMaxPower float64
	var peakRecords int

	runSpike := Spike{}
	inSpike := false
	runMaint := MaintenanceAlert{}
	inMaint := false
	prev := TelemetryRecord{}

	for _, rec := range records {
		facilities[rec.FacilityID] = struct{}{}
		totalEnergy += rec.EnergyKWh
		totalCarbon += rec.CarbonKG

		isPeak := rec.IsPeakHour
		if useCustomPeak {
			if ts, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
				minutes := ts.Hour()*60 + ts.Minute()
				isPeak = inWindow(minutes, peakStartMin, peakEndMin, wraps)
			} else {
				isPeak = false
			}
		}
		if isPeak {
			peakRecords++
			peakEnergy += rec.EnergyKWh
			if rec.PowerKW > peakMaxPower {
				peakMaxPower = rec.PowerKW
			}
		}

		// Critical spike run detection by ratio-to-baseline magnitude.
		spikeActive := rec.RatioToBaseline >= minSpikeRatio
		if spikeActive && inSpike && rec.FacilityID == prev.FacilityID {
			runSpike.End = rec.Timestamp
			runSpike.DurationMinutes++
			if rec.PowerKW > runSpike.MaxPowerKW {
				runSpike.MaxPowerKW = rec.PowerKW
			}
			if rec.RatioToBaseline > runSpike.MaxRatio {
				runSpike.MaxRatio = rec.RatioToBaseline
			}
		} else {
			if inSpike {
				result.CriticalSpikes = append(result.CriticalSpikes, runSpike)
				inSpike = false
			}
			if spikeActive {
				runSpike = Spike{
					FacilityID:      rec.FacilityID,
					FacilityName:    rec.FacilityName,
					EquipmentID:     rec.EquipmentID,
					Start:           rec.Timestamp,
					End:             rec.Timestamp,
					DurationMinutes: 1,
					MaxPowerKW:      rec.PowerKW,
					MaxRatio:        rec.RatioToBaseline,
					Severity:        severityLabel(rec.AnomalySeverity),
				}
				inSpike = true
			}
		}

		// Predictive maintenance run detection (micro-surges).
		sameEquipment := rec.EquipmentID == prev.EquipmentID
		if rec.AnomalyFlag == "micro_surge" && inMaint && rec.FacilityID == prev.FacilityID && sameEquipment {
			runMaint.End = rec.Timestamp
			runMaint.DurationMinutes++
			if rec.RatioToBaseline > runMaint.MaxRatio {
				runMaint.MaxRatio = rec.RatioToBaseline
			}
		} else {
			if inMaint {
				result.MaintenanceAlerts = append(result.MaintenanceAlerts, finalizeAlert(runMaint))
				inMaint = false
			}
			if rec.AnomalyFlag == "micro_surge" {
				runMaint = MaintenanceAlert{
					FacilityID:      rec.FacilityID,
					FacilityName:    rec.FacilityName,
					EquipmentID:     rec.EquipmentID,
					Start:           rec.Timestamp,
					End:             rec.Timestamp,
					DurationMinutes: 1,
					MaxRatio:        rec.RatioToBaseline,
					Severity:        severityLabel(rec.AnomalySeverity),
				}
				inMaint = true
			}
		}

		prev = rec
	}

	if inSpike {
		result.CriticalSpikes = append(result.CriticalSpikes, runSpike)
	}
	if inMaint {
		result.MaintenanceAlerts = append(result.MaintenanceAlerts, finalizeAlert(runMaint))
	}

	result.FacilityCount = len(facilities)
	result.TotalEnergyKWh = round4(totalEnergy)
	result.TotalCarbonKG = round4(totalCarbon)
	share := 0.0
	if totalEnergy > 0 {
		share = peakEnergy / totalEnergy * 100
	}
	windowLabel := "18:00-22:00"
	if useCustomPeak {
		windowLabel = fmt.Sprintf("%s-%s", cfg.PeakStart, cfg.PeakEnd)
	}
	result.PeakMetrics = PeakMetrics{
		PeakWindow:            windowLabel,
		TotalEnergyKWh:        round4(peakEnergy),
		PeakHourRecords:       peakRecords,
		MaxDemandKW:           round4(peakMaxPower),
		ShareOfTotalEnergyPct: round4(share),
	}
	return result
}

// parseClock parses an "HH:MM" 24-hour clock string into minutes since
// midnight.
func parseClock(value string) (int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("invalid clock time %q; use HH:MM", value)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// inWindow reports whether minutes falls inside [start, end). A window where
// start > end wraps across midnight.
func inWindow(minutes, start, end int, wraps bool) bool {
	if wraps {
		return minutes >= start || minutes < end
	}
	return minutes >= start && minutes < end
}

func severityLabel(raw string) string {
	if raw == "" {
		return "critical"
	}
	return raw
}

func finalizeAlert(alert MaintenanceAlert) MaintenanceAlert {
	alert.Recommendation = fmt.Sprintf(
		"Gradual micro-surge detected on %s at %s over %d minutes (max ratio %.2f). "+
			"Inspect for bearing wear, insulation degradation, or wiring leakage. "+
			"Schedule predictive maintenance outside peak tariff hours.",
		alert.EquipmentID, alert.FacilityName, alert.DurationMinutes, alert.MaxRatio,
	)
	return alert
}
