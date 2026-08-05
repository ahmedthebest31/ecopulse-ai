export interface TelemetryRecord {
  timestamp: string
  facility_id: string
  facility_name: string
  facility_type: string
  equipment_id: string
  power_kw: number
  baseline_kw: number
  ratio_to_baseline: number
  energy_kwh: number
  voltage_v: number
  current_a: number
  power_factor: number
  frequency_hz: number
  is_peak_hour: boolean
  anomaly_flag: string
  anomaly_severity: string
  carbon_kg: number
}

export type AnomalyFlag = 'none' | 'forced_spike' | 'micro_surge'

export interface Tier {
  name: string
  lower_kwh: number
  upper_kwh: number
  rate_egp: number
}

export interface Spike {
  facility_id: string
  facility_name: string
  equipment_id: string
  start: string
  end: string
  duration_minutes: number
  max_power_kw: number
  max_ratio_to_baseline: number
  severity: string
}

export interface MaintenanceAlert {
  facility_id: string
  facility_name: string
  equipment_id: string
  start: string
  end: string
  duration_minutes: number
  max_ratio_to_baseline: number
  severity: string
  recommendation: string
}

export interface PeakMetrics {
  peak_window: string
  total_energy_kwh: number
  peak_hour_records: number
  max_demand_kw: number
  share_of_total_energy_pct: number
}

export interface AnalyticsResult {
  total_records: number
  facility_count: number
  spike_threshold_percent: number
  critical_spikes: Spike[]
  maintenance_alerts: MaintenanceAlert[]
  peak_metrics: PeakMetrics
  total_energy_kwh: number
  total_carbon_kg: number
}

export interface TelemetryResponse {
  total_records: number
  count: number
  filters: { facility_id: string; start: string; end: string }
  records: TelemetryRecord[]
}

export interface TariffTierLine {
  name: string
  label: string
  lower_kwh: number
  upper_kwh: number
  kwh_used: number
  rate_egp: number
  cost_egp: number
  cost_usd: number
}

export interface TariffBreakdown {
  mode: 'flat' | 'tiered'
  kwh: number
  currency: string
  total_cost_egp: number
  total_cost_usd: number
  effective_rate_per_kwh_egp: number
  tiers: TariffTierLine[]
}

export interface TariffRequest {
  kwh: number
  mode: 'flat' | 'tiered'
  flat_rate_egp?: number
  tiers?: Tier[]
  usd_per_egp?: number
}

export interface ReportResult {
  locale: string
  summary: string
  source: 'gemini' | 'fallback'
  model?: string
  warning?: string
}

export interface GeminiStatusResult {
  has_valid_env_key: boolean
}
