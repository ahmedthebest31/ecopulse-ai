// Package analytics aggregates telemetry records into executive metrics:
// critical consumption spikes, predictive maintenance alerts, and peak-hour
// load statistics. It operates on the records produced by the EcoPulse AI
// data generator.
package analytics

import (
	"fmt"
	"math"
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

// Config controls spike detection thresholds.
type Config struct {
	SpikeThresholdPercent float64 // consumption above baseline that marks a spike
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
// records with the same anomaly flag and facility.
func Analyze(records []TelemetryRecord, cfg Config) Result {
	if cfg.SpikeThresholdPercent <= 0 {
		cfg.SpikeThresholdPercent = 30
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

		if rec.IsPeakHour {
			peakRecords++
			peakEnergy += rec.EnergyKWh
			if rec.PowerKW > peakMaxPower {
				peakMaxPower = rec.PowerKW
			}
		}

		// Critical spike run detection (forced spikes).
		if rec.AnomalyFlag == "forced_spike" && inSpike && rec.FacilityID == prev.FacilityID {
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
			if rec.AnomalyFlag == "forced_spike" {
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
	result.PeakMetrics = PeakMetrics{
		PeakWindow:            "18:00-22:00",
		TotalEnergyKWh:        round4(peakEnergy),
		PeakHourRecords:       peakRecords,
		MaxDemandKW:           round4(peakMaxPower),
		ShareOfTotalEnergyPct: round4(share),
	}
	return result
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
