"""EcoPulse AI telemetry simulator.

Generates a realistic 24-hour time-series dataset at 1-minute resolution for
multiple facilities. Each facility record includes:

- Normal baseline consumption with day/night profiles.
- Night-shift standby consumption.
- Peak-hour surge between 18:00 and 22:00 with smooth ramps.
- Voltage fluctuations, sag during peak hours, and frequency jitter.
- Forced spike anomalies (boost factor 30%+ over baseline) -> critical alerts.
- Micro-surges (gradual, non-periodic 5-15% boosts) -> predictive maintenance.

Outputs a JSON file and a CSV file under the configured output directory.

Timezone note: timestamps are produced as timezone-aware values with a fixed
+03:00 offset (Egypt Standard Time, Africa/Cairo). The stdlib `zoneinfo`
database is not used because it requires the third-party `tzdata` package on
Windows; the fixed offset keeps the generator dependency-free and offline.
"""

from __future__ import annotations

import csv
import json
import math
import random
import sys
from datetime import UTC, datetime, timedelta, timezone
from pathlib import Path
from typing import Any, TypedDict

SCRIPT_DIR = Path(__file__).resolve().parent
DEFAULT_CONFIG_PATH = SCRIPT_DIR / "config.json"

MINUTES_PER_HOUR = 60
MINUTES_PER_DAY = 24 * MINUTES_PER_HOUR


class Facility(TypedDict):
    id: str
    name: str
    type: str
    base_power_kw: float
    equipment: list[str]


def load_config(path: Path) -> dict[str, Any]:
    """Load and validate the generator configuration file."""
    with path.open("r", encoding="utf-8") as handle:
        config = json.load(handle)
    facilities = config.get("facilities", [])
    if not facilities:
        raise ValueError("config.json must define at least one facility")
    if not all("base_power_kw" in f and f["base_power_kw"] > 0 for f in facilities):
        raise ValueError("every facility must define a positive base_power_kw")
    return config


def parse_clock(value: str) -> int:
    """Convert an 'HH:MM' string into minutes since midnight."""
    hours_text, minutes_text = value.split(":")
    return int(hours_text) * MINUTES_PER_HOUR + int(minutes_text)


def build_timestamps(start_date: str, hours: int, interval_minutes: int, utc_offset: str) -> list[datetime]:
    """Build a list of timezone-aware timestamps covering the simulation window."""
    offset_hours, offset_minutes = (int(part) for part in utc_offset[1:].split(":"))
    if utc_offset.startswith("-"):
        offset_hours = -offset_hours
    tz = timezone(timedelta(hours=offset_hours, minutes=offset_minutes))
    start = datetime.fromisoformat(start_date).replace(tzinfo=tz)
    total_minutes = hours * MINUTES_PER_HOUR
    return [start + timedelta(minutes=index * interval_minutes) for index in range(total_minutes)]


def is_peak_hour(current: datetime, peak_start: int, peak_end: int) -> bool:
    """Return True when the given timestamp falls inside the peak window."""
    minute_of_day = current.hour * MINUTES_PER_HOUR + current.minute
    return peak_start <= minute_of_day < peak_end


def day_profile_factor(minute_of_day: int, config: dict[str, Any], facility_random: random.Random) -> float:
    """Compute the baseline consumption multiplier for the given minute of day.

    The profile combines night standby, business-hours load, and a peak surge
    window with smooth linear ramps between the states.
    """
    patterns = config["patterns"]
    night_factor = facility_random.uniform(*patterns["night_standby_factor_range"])
    peak_factor = facility_random.uniform(*patterns["peak_surge_factor_range"])

    morning_start = parse_clock(patterns["morning_ramp_start"])
    morning_end = parse_clock(patterns["morning_ramp_end"])
    evening_start = parse_clock(patterns["evening_ramp_start"])
    evening_end = parse_clock(patterns["evening_ramp_end"])
    peak_end = parse_clock(patterns["peak_window"]["end"])
    night_start = parse_clock(patterns["night_standby_window"]["start"])
    night_end = parse_clock(patterns["night_standby_window"]["end"])

    # Night-shift standby window 22:00 -> 06:00 (wraps midnight).
    if minute_of_day >= night_start or minute_of_day < night_end:
        return night_factor

    # Morning ramp 05:00 -> 07:00. Minutes 05:00-06:00 are handled above as
    # night standby, so this branch effectively covers 06:00-07:00.
    if morning_start <= minute_of_day < morning_end:
        progress = (minute_of_day - morning_start) / (morning_end - morning_start)
        return night_factor + (1.0 - night_factor) * progress

    # Business-hours baseline 07:00 -> 17:00.
    if minute_of_day < evening_start:
        return 1.0

    # Ramp into the peak window 17:00 -> 18:00.
    if minute_of_day < evening_end:
        progress = (minute_of_day - evening_start) / (evening_end - evening_start)
        return 1.0 + (peak_factor - 1.0) * progress

    # Peak window 18:00 -> 22:00.
    if minute_of_day < peak_end:
        return peak_factor

    # Safety fallback; unreachable because night standby covers >= 22:00.
    return night_factor


def build_anomaly_events(facility: Facility, total_minutes: int, config: dict[str, Any], rng: random.Random) -> dict[int, dict[str, Any]]:
    """Pre-compute forced spikes and micro-surge windows for one facility.

    Returns a mapping of minute index -> event descriptor. Events never
    overlap: each event consumes its minutes in an occupancy set.
    """
    anomalies = config["anomalies"]
    equipment = facility["equipment"]
    occupied: set[int] = set()
    events: dict[int, dict[str, Any]] = {}

    forced = anomalies["forced_spike"]
    for _ in range(forced["count_per_facility"]):
        duration = rng.randint(*forced["duration_minutes_range"])
        max_start = total_minutes - duration
        if max_start <= 0:
            continue
        for _attempt in range(200):
            start = rng.randint(0, max_start)
            window = range(start, start + duration)
            if not occupied.intersection(window):
                break
        else:
            continue
        boost = rng.uniform(*forced["boost_factor_range"])
        machine = rng.choice(equipment)
        for minute_index in window:
            occupied.add(minute_index)
            events[minute_index] = {"type": "forced_spike", "equipment": machine, "boost": boost}

    micro = anomalies["micro_surge"]
    for _ in range(micro["count_per_facility"]):
        duration = rng.randint(*micro["window_minutes_range"])
        max_start = total_minutes - duration
        if max_start <= 0:
            continue
        for _attempt in range(200):
            start = rng.randint(0, max_start)
            window = range(start, start + duration)
            if not occupied.intersection(window):
                break
        else:
            continue
        machine = rng.choice(equipment)
        low, high = micro["boost_factor_range"]
        per_minute_boosts: list[float] = []
        current = rng.uniform(low, (low + high) / 2.0)
        for _minute in window:
            current = min(high, max(low, current + rng.uniform(-0.02, 0.025)))
            per_minute_boosts.append(current)
        for minute_index, boost in zip(window, per_minute_boosts, strict=False):
            occupied.add(minute_index)
            events[minute_index] = {"type": "micro_surge", "equipment": machine, "boost": boost}

    return events


def simulate_facility(
    facility: Facility,
    timestamps: list[datetime],
    config: dict[str, Any],
    rng: random.Random,
) -> list[dict[str, Any]]:
    """Simulate one facility across the full 24-hour window and return records."""
    patterns = config["patterns"]
    carbon_factor = config["carbon_factor_kg_per_kwh"]
    base_power = facility["base_power_kw"]
    peak_start = parse_clock(patterns["peak_window"]["start"])
    peak_end = parse_clock(patterns["peak_window"]["end"])
    nominal_voltage = patterns["voltage_nominal_v"]
    voltage_fluctuation_pct = patterns["voltage_fluctuation_pct"]
    voltage_sag_pct = patterns["voltage_sag_during_peak_pct"]
    nominal_frequency = patterns["frequency_nominal_hz"]
    frequency_jitter_pct = patterns["frequency_jitter_pct"]
    power_factor_range = patterns["power_factor_range"]

    total_minutes = len(timestamps)
    events = build_anomaly_events(facility, total_minutes, config, rng)
    per_minute_peak_factor = [day_profile_factor(t.hour * MINUTES_PER_HOUR + t.minute, config, rng) for t in timestamps]

    records: list[dict[str, Any]] = []
    for minute_index, timestamp in enumerate(timestamps):
        in_peak = is_peak_hour(timestamp, peak_start, peak_end)

        baseline_kw = base_power * per_minute_peak_factor[minute_index]
        baseline_kw *= 1.0 + rng.uniform(-0.02, 0.02)

        event = events.get(minute_index)
        anomaly_flag = "none"
        anomaly_severity = ""
        equipment_id = ""
        power_kw = baseline_kw
        if event is not None:
            anomaly_flag = event["type"]
            equipment_id = event["equipment"]
            power_kw = baseline_kw * event["boost"]
            anomaly_severity = "critical" if event["type"] == "forced_spike" else "predictive_maintenance"

        energy_kwh = power_kw / MINUTES_PER_HOUR

        voltage_sag = (voltage_sag_pct / 100.0) if in_peak else 0.0
        voltage_noise = rng.uniform(-voltage_fluctuation_pct, voltage_fluctuation_pct) / 100.0
        voltage_v = nominal_voltage * (1.0 + voltage_noise - voltage_sag)

        frequency_hz = nominal_frequency * (1.0 + rng.uniform(-frequency_jitter_pct, frequency_jitter_pct) / 100.0)
        power_factor = rng.uniform(*power_factor_range)
        current_a = (power_kw * 1000.0) / (math.sqrt(3.0) * voltage_v * power_factor)

        ratio_to_baseline = power_kw / baseline_kw if baseline_kw > 0 else 1.0

        records.append(
            {
                "timestamp": timestamp.isoformat(timespec="seconds"),
                "facility_id": facility["id"],
                "facility_name": facility["name"],
                "facility_type": facility["type"],
                "equipment_id": equipment_id,
                "power_kw": round(power_kw, 3),
                "baseline_kw": round(baseline_kw, 3),
                "ratio_to_baseline": round(ratio_to_baseline, 4),
                "energy_kwh": round(energy_kwh, 5),
                "voltage_v": round(voltage_v, 2),
                "current_a": round(current_a, 2),
                "power_factor": round(power_factor, 3),
                "frequency_hz": round(frequency_hz, 3),
                "is_peak_hour": in_peak,
                "anomaly_flag": anomaly_flag,
                "anomaly_severity": anomaly_severity,
                "carbon_kg": round(energy_kwh * carbon_factor, 5),
            }
        )

    return records


def summarize_dataset(records: list[dict[str, Any]], config: dict[str, Any]) -> dict[str, Any]:
    """Compute dataset-level statistics used in the JSON header."""
    forced_spikes = sum(1 for record in records if record["anomaly_flag"] == "forced_spike")
    micro_surges = sum(1 for record in records if record["anomaly_flag"] == "micro_surge")
    peak_records = sum(1 for record in records if record["is_peak_hour"])
    total_energy_kwh = sum(record["energy_kwh"] for record in records)
    return {
        "total_records": len(records),
        "facility_count": len(config["facilities"]),
        "forced_spike_records": forced_spikes,
        "micro_surge_records": micro_surges,
        "peak_hour_records": peak_records,
        "total_energy_kwh": round(total_energy_kwh, 2),
        "total_carbon_kg": round(sum(record["carbon_kg"] for record in records), 2),
    }


def write_json(records: list[dict[str, Any]], config: dict[str, Any], destination: Path) -> None:
    """Write the telemetry dataset as a structured JSON payload."""
    simulation = config["simulation"]
    output = {
        "schema_version": "1.0",
        "generated_at": datetime.now(UTC).isoformat(timespec="seconds"),
        "interval_minutes": simulation["interval_minutes"],
        "hours": simulation["hours"],
        "timezone": simulation["timezone"],
        "utc_offset": simulation["utc_offset"],
        "spike_threshold_percent": config["anomalies"]["spike_threshold_percent"],
        "carbon_factor_kg_per_kwh": config["carbon_factor_kg_per_kwh"],
        "peak_window": config["patterns"]["peak_window"],
        "summary": summarize_dataset(records, config),
        "records": records,
    }
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("w", encoding="utf-8") as handle:
        json.dump(output, handle, ensure_ascii=False, indent=2)


def write_csv(records: list[dict[str, Any]], destination: Path) -> None:
    """Write the telemetry dataset as a flat CSV file."""
    fieldnames = [
        "timestamp",
        "facility_id",
        "facility_name",
        "facility_type",
        "equipment_id",
        "power_kw",
        "baseline_kw",
        "ratio_to_baseline",
        "energy_kwh",
        "voltage_v",
        "current_a",
        "power_factor",
        "frequency_hz",
        "is_peak_hour",
        "anomaly_flag",
        "anomaly_severity",
        "carbon_kg",
    ]
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(records)


def main() -> int:
    """Entry point: generate the dataset and write both output files."""
    config_path = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_CONFIG_PATH
    config = load_config(config_path)

    simulation = config["simulation"]
    output_cfg = config["output"]

    timestamps = build_timestamps(
        simulation["start_date"],
        simulation["hours"],
        simulation["interval_minutes"],
        simulation["utc_offset"],
    )

    all_records: list[dict[str, Any]] = []
    for facility in config["facilities"]:
        facility_rng = random.Random(f"{simulation['random_seed']}-{facility['id']}")
        all_records.extend(simulate_facility(facility, timestamps, config, facility_rng))

    output_dir = SCRIPT_DIR / output_cfg["dir"]
    json_path = output_dir / output_cfg["json_file"]
    csv_path = output_dir / output_cfg["csv_file"]

    write_json(all_records, config, json_path)
    write_csv(all_records, csv_path)

    summary = summarize_dataset(all_records, config)
    print(f"Generated {summary['total_records']} telemetry records.")
    print(f"Facilities: {summary['facility_count']}")
    print(f"Forced spike records: {summary['forced_spike_records']}")
    print(f"Micro-surge records: {summary['micro_surge_records']}")
    print(f"Peak-hour records: {summary['peak_hour_records']}")
    print(f"Total energy: {summary['total_energy_kwh']} kWh")
    print(f"Total carbon: {summary['total_carbon_kg']} kg CO2")
    print(f"JSON written to: {json_path}")
    print(f"CSV written to: {csv_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
