"""EcoPulse AI telemetry simulator.

Generates a realistic time-series dataset at a configurable interval for
multiple facilities. Each facility record includes:

- Normal baseline consumption with day/night profiles. The night-standby and
  peak-surge multipliers are drawn once per facility per run, so profiles stay
  smooth instead of re-randomizing every minute.
- Night-shift standby consumption.
- Peak-hour surge between 18:00 and 22:00 with smooth ramps.
- Voltage fluctuations, sag during peak hours, and frequency jitter.
- Forced spike anomalies (boost factor 30%+ over baseline) -> critical alerts.
- Micro-surges (gradual, non-periodic 5-15% boosts) -> predictive maintenance.

Outputs a JSON file and a CSV file under the configured output directory.
Writes are atomic: each file lands via a temporary file plus os.replace, so a
crash mid-write can never leave a truncated dataset behind.

Timezone note: timestamps are produced as timezone-aware values with a fixed
+03:00 offset (Egypt Standard Time, Africa/Cairo). The stdlib `zoneinfo`
database is not used because it requires the third-party `tzdata` package on
Windows; the fixed offset keeps the generator dependency-free and offline.
"""

from __future__ import annotations

import argparse
import csv
import io
import json
import math
import os
import random
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


def _require_positive(container: dict[str, Any], key: str, label: str) -> None:
    value = container.get(key)
    if not isinstance(value, (int, float)) or isinstance(value, bool) or value <= 0:
        raise ValueError(f"{label}.{key} must be a positive number, got {value!r}")


def _require_range(container: dict[str, Any], key: str, label: str) -> None:
    value = container.get(key)
    if (
        not isinstance(value, list)
        or len(value) != 2
        or any(isinstance(v, bool) or not isinstance(v, (int, float)) for v in value)
        or value[0] > value[1]
    ):
        raise ValueError(f"{label}.{key} must be a [low, high] number pair with low <= high, got {value!r}")


def _require_clock(container: dict[str, Any], key: str, label: str) -> None:
    value = container.get(key)
    if not isinstance(value, str):
        raise ValueError(f"{label}.{key} must be an HH:MM string, got {value!r}")
    parse_clock(value)  # raises ValueError with a precise message when invalid


def load_config(path: Path) -> dict[str, Any]:
    """Load and deeply validate the generator configuration file."""
    with path.open("r", encoding="utf-8") as handle:
        config = json.load(handle)
    if not isinstance(config, dict):
        raise ValueError("config root must be a JSON object")

    simulation = config.get("simulation")
    if not isinstance(simulation, dict):
        raise ValueError("config.json must define a 'simulation' object")
    _require_positive(simulation, "hours", "simulation")
    _require_positive(simulation, "interval_minutes", "simulation")
    if not isinstance(simulation.get("random_seed"), int) or isinstance(simulation["random_seed"], bool):
        raise ValueError("simulation.random_seed must be an integer")
    start_date = simulation.get("start_date")
    if not isinstance(start_date, str):
        raise ValueError("simulation.start_date must be an ISO date string like 2026-08-03")
    try:
        datetime.fromisoformat(start_date)
    except ValueError as exc:
        raise ValueError(f"simulation.start_date is not a valid ISO date: {start_date!r}") from exc
    offset = simulation.get("utc_offset")
    if (
        not isinstance(offset, str)
        or len(offset) != 6
        or offset[0] not in "+-"
        or offset[3] != ":"
        or not offset[1:3].isdigit()
        or not offset[4:].isdigit()
    ):
        raise ValueError("simulation.utc_offset must look like +HH:MM or -HH:MM")

    patterns = config.get("patterns")
    if not isinstance(patterns, dict):
        raise ValueError("config.json must define a 'patterns' object")
    peak_window = patterns.get("peak_window")
    if not isinstance(peak_window, dict):
        raise ValueError("patterns.peak_window must be an object with start/end clock times")
    _require_clock(peak_window, "start", "patterns.peak_window")
    _require_clock(peak_window, "end", "patterns.peak_window")
    night_window = patterns.get("night_standby_window")
    if not isinstance(night_window, dict):
        raise ValueError("patterns.night_standby_window must be an object with start/end clock times")
    _require_clock(night_window, "start", "patterns.night_standby_window")
    _require_clock(night_window, "end", "patterns.night_standby_window")
    for key in ("morning_ramp_start", "morning_ramp_end", "evening_ramp_start", "evening_ramp_end"):
        _require_clock(patterns, key, "patterns")
    _require_range(patterns, "night_standby_factor_range", "patterns")
    _require_range(patterns, "peak_surge_factor_range", "patterns")
    _require_range(patterns, "power_factor_range", "patterns")
    _require_positive(patterns, "voltage_nominal_v", "patterns")
    _require_positive(patterns, "frequency_nominal_hz", "patterns")

    anomalies = config.get("anomalies")
    if not isinstance(anomalies, dict):
        raise ValueError("config.json must define an 'anomalies' object")
    _require_positive(anomalies, "spike_threshold_percent", "anomalies")
    for group in ("forced_spike", "micro_surge"):
        section = anomalies.get(group)
        if not isinstance(section, dict):
            raise ValueError(f"anomalies.{group} must be an object")
        count = section.get("count_per_facility")
        if not isinstance(count, int) or isinstance(count, bool) or count < 0:
            raise ValueError(f"anomalies.{group}.count_per_facility must be a non-negative integer")
        _require_range(section, "boost_factor_range", f"anomalies.{group}")

    _require_positive(config, "carbon_factor_kg_per_kwh", "config")

    output_cfg = config.get("output")
    if not isinstance(output_cfg, dict):
        raise ValueError("config.json must define an 'output' object")
    for key in ("dir", "json_file", "csv_file"):
        if not isinstance(output_cfg.get(key), str) or not output_cfg[key]:
            raise ValueError(f"output.{key} must be a non-empty string")

    facilities = config.get("facilities")
    if not facilities:
        raise ValueError("config.json must define at least one facility")
    seen_ids: set[str] = set()
    for facility in facilities:
        if not isinstance(facility, dict):
            raise ValueError("every facility must be an object")
        facility_id = facility.get("id")
        if not isinstance(facility_id, str) or not facility_id:
            raise ValueError("every facility needs a non-empty string id")
        if facility_id in seen_ids:
            raise ValueError(f"duplicate facility id: {facility_id}")
        seen_ids.add(facility_id)
        for key in ("name", "type"):
            if not isinstance(facility.get(key), str) or not facility[key]:
                raise ValueError(f"facility {facility_id}.{key} must be a non-empty string")
        _require_positive(facility, "base_power_kw", f"facility {facility_id}")
        equipment = facility.get("equipment")
        if not isinstance(equipment, list) or not equipment or not all(isinstance(e, str) and e for e in equipment):
            raise ValueError(f"facility {facility_id}.equipment must be a non-empty list of names")
    return config


def parse_clock(value: str) -> int:
    """Convert an 'HH:MM' string into minutes since midnight.

    Raises ValueError on malformed strings or out-of-range components
    (e.g. "25:99"), so bad configuration fails fast instead of silently
    producing a broken profile.
    """
    try:
        hours_text, minutes_text = value.split(":")
        hours, minutes = int(hours_text), int(minutes_text)
    except (AttributeError, ValueError) as exc:
        raise ValueError(f"invalid clock time {value!r}; expected HH:MM") from exc
    if not (0 <= hours < 24 and 0 <= minutes < 60):
        raise ValueError(f"clock time {value!r} out of range; expected hours 00-23 and minutes 00-59")
    return hours * MINUTES_PER_HOUR + minutes


def build_timestamps(start_date: str, hours: int, interval_minutes: int, utc_offset: str) -> list[datetime]:
    """Build a list of timezone-aware timestamps covering the simulation window.

    The count honors interval_minutes: hours * 60 // interval samples spread
    across the window (1440 timestamps for the default 24h @ 1-minute).
    """
    offset_hours, offset_minutes = (int(part) for part in utc_offset[1:].split(":"))
    if utc_offset.startswith("-"):
        offset_hours = -offset_hours
    tz = timezone(timedelta(hours=offset_hours, minutes=offset_minutes))
    start = datetime.fromisoformat(start_date).replace(tzinfo=tz)
    total_count = (hours * MINUTES_PER_HOUR) // interval_minutes
    if total_count <= 0:
        raise ValueError("simulation window produces no timestamps; check hours and interval_minutes")
    return [start + timedelta(minutes=index * interval_minutes) for index in range(total_count)]


def is_peak_hour(current: datetime, peak_start: int, peak_end: int) -> bool:
    """Return True when the given timestamp falls inside the peak window."""
    minute_of_day = current.hour * MINUTES_PER_HOUR + current.minute
    return peak_start <= minute_of_day < peak_end


def day_profile_factor(minute_of_day: int, night_factor: float, peak_factor: float, patterns: dict[str, Any]) -> float:
    """Compute the baseline consumption multiplier for the given minute of day.

    The profile combines night standby, business-hours load, and a peak surge
    window with smooth linear ramps between the states. night_factor and
    peak_factor are sampled once per facility per run by the caller so the
    curve stays smooth instead of re-randomizing every minute.
    """
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


def build_anomaly_events(
    facility: Facility, total_minutes: int, config: dict[str, Any], rng: random.Random
) -> dict[int, dict[str, Any]]:
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
    interval_minutes: int,
    config: dict[str, Any],
    rng: random.Random,
) -> list[dict[str, Any]]:
    """Simulate one facility across the full window and return records."""
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

    # Sample the day-profile multipliers once per facility so each curve is
    # smooth; per-minute sampling produced jagged, unrealistic profiles.
    night_factor = rng.uniform(*patterns["night_standby_factor_range"])
    peak_factor = rng.uniform(*patterns["peak_surge_factor_range"])

    records: list[dict[str, Any]] = []
    for minute_index, timestamp in enumerate(timestamps):
        minute_of_day = timestamp.hour * MINUTES_PER_HOUR + timestamp.minute
        in_peak = is_peak_hour(timestamp, peak_start, peak_end)

        baseline_kw = base_power * day_profile_factor(minute_of_day, night_factor, peak_factor, patterns)
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

        energy_kwh = power_kw * interval_minutes / MINUTES_PER_HOUR

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


def _atomic_write(destination: Path, content: str) -> None:
    """Write text atomically: temp file first, then os.replace into place.

    A crash mid-write leaves the previous file intact instead of a truncated
    dataset that would break backend startup.
    """
    destination.parent.mkdir(parents=True, exist_ok=True)
    temp_path = destination.with_name(destination.name + ".tmp")
    try:
        with temp_path.open("w", encoding="utf-8", newline="") as handle:
            handle.write(content)
        os.replace(temp_path, destination)
    except OSError:
        temp_path.unlink(missing_ok=True)
        raise


def write_json(
    records: list[dict[str, Any]],
    config: dict[str, Any],
    summary: dict[str, Any],
    destination: Path,
) -> None:
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
        "summary": summary,
        "records": records,
    }
    _atomic_write(destination, json.dumps(output, ensure_ascii=False, indent=2))


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
    buffer = io.StringIO(newline="")
    writer = csv.DictWriter(buffer, fieldnames=fieldnames)
    writer.writeheader()
    writer.writerows(records)
    _atomic_write(destination, buffer.getvalue())


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse CLI overrides; every default preserves config.json behavior."""
    parser = argparse.ArgumentParser(
        description="EcoPulse AI telemetry dataset generator.",
    )
    parser.add_argument(
        "--config",
        type=Path,
        default=DEFAULT_CONFIG_PATH,
        help=f"path to the generator config file (default: {DEFAULT_CONFIG_PATH})",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=None,
        help="override simulation.random_seed for reproducible variants",
    )
    parser.add_argument(
        "--out-dir",
        type=Path,
        default=None,
        help="override output.dir (absolute path, or relative to the generator folder)",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    """Entry point: generate the dataset and write both output files."""
    args = parse_args(argv)
    config = load_config(args.config)

    simulation = config["simulation"]
    output_cfg = config["output"]
    if args.seed is not None:
        simulation["random_seed"] = args.seed

    timestamps = build_timestamps(
        simulation["start_date"],
        simulation["hours"],
        simulation["interval_minutes"],
        simulation["utc_offset"],
    )

    all_records: list[dict[str, Any]] = []
    for facility in config["facilities"]:
        facility_rng = random.Random(f"{simulation['random_seed']}-{facility['id']}")
        all_records.extend(
            simulate_facility(facility, timestamps, simulation["interval_minutes"], config, facility_rng)
        )

    output_dir = Path(args.out_dir) if args.out_dir is not None else SCRIPT_DIR / output_cfg["dir"]
    if not output_dir.is_absolute():
        output_dir = SCRIPT_DIR / output_dir
    json_path = output_dir / output_cfg["json_file"]
    csv_path = output_dir / output_cfg["csv_file"]

    summary = summarize_dataset(all_records, config)
    write_json(all_records, config, summary, json_path)
    write_csv(all_records, csv_path)

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
