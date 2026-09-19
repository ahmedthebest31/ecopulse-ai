# EcoPulse AI - Factory BOM (hardware_sim)

Bill of materials, wiring notes and safety guidance for the physical device
that the `hardware-sim` Wokwi project simulates. Every component below maps
one-to-one to a part in `diagram.json` and a pin in `sketch.ino`.

Prices are indicative Egyptian-market street prices (2026) and must be
re-verified with the supplier before purchase. Stock at El Nouh Electrical
and Ram Electronics changes weekly - always call first.

## Wiring model

The physical device mirrors the simulator:

- SCT-013-000 current transformer is the potentiometer (ADC input GPIO34).
- ZMPT101B voltage sensor supplies the 380 V line reading used in the kW
  calculation (constant VOLTAGE_RMS in the firmware, verified in the field).
- Green LED = "OK / normal" beacon (GPIO2). Red LED = "WARNING / CRITICAL"
  beacon (GPIO4).
- Passive buzzer = audible alarm (GPIO17).
- Relay module drives the non-critical load (GPIO32). The load is wired on
  COM-NC so it stays ON by default and is shed only when the backend
  commands CRITICAL load shedding.

## Parts list

1. ESP32 DevKit (ESP32-WROOM-32), 30-pin. Approx 250-350 EGP. This is the
   simulated wokwi-esp32-devkit-v1.

2. SCT-013-000 split-core current transformer, 100 A max, 0-1 V output.
   Approx 150-250 EGP. Non-invasive clamp around one phase conductor.
   This is the simulated potentiometer (ADC input, GPIO34).

3. ZMPT101B voltage transformer module, 0-250 V AC. Approx 120-180 EGP.
   Feed it the 380 V phase via a fused, rated step-down tap. Output must be
   centered at 2.5 V (bias divider) so the ESP32 ADC never sees a negative
   excursion.

4. 5 V single-channel optocoupler relay module (SRD-05VDC-SL-C class),
   low-level trigger, jumper set to LOW. Approx 40-70 EGP. This is the
   simulated wokwi-relay-module in its default npn (active-high contact)
   behavior: COM-NC closed when IN is high or floating, COM-NO when IN is
   low. Load on COM-NC therefore stays ON during boot (fail-safe) and is
   shed only at CRITICAL.

5. DIN-rail power supply, 380 V AC input to 5 V DC, at least 3 A.
   Approx 250-400 EGP. Powers the ESP32 and the relay module (relay coil
   peaks ~70 mA; leave the rest for sensors).

6. 220 ohm 0.25 W resistors, 5 pcs. Approx 5 EGP. Current limiters for the
   two indicator LEDs; same value as the simulator resistors.

7. Bias resistors for the CT / voltage mid-point (e.g., 2.2 k value, 4 pcs)
   plus a 10 uF electrolytic for smoothing. Approx 15 EGP.

8. Passive buzzer 5 V (e.g., 3-pin or 2-pin piezo). Approx 15-30 EGP.
   This is the simulated wokwi-buzzer.

9. Green LED and red LED, 5 mm. Approx 5 EGP total. These are the simulated
   wokwi-led OK/WARN lamps.

10. DIN enclosure with terminal blocks, DIN rail, cable glands, fuses.
    Approx 150-300 EGP.

11. Jumper wires, heat-shrink, cable ties, 0.75 mm2 flexible wiring.
    Approx 50-100 EGP.

Estimated total: 1200-1800 EGP per unit, excluding the electrician labour for
the main-panel installation.

## Wiring summary

- GPIO34 (input only) <- SCT-013-000 output through the bias mid-point.
- ESP32 ADC reference must never exceed 3.6 V. Fuse and clamp the sensor
  secondary against a CT opening fault.
- GPIO2 -> 220 R -> green LED -> GND.
- GPIO4 -> 220 R -> red LED -> GND.
- GPIO17 -> buzzer positive; buzzer negative -> GND.
- GPIO32 -> relay module IN. Relay VCC -> 5 V, GND -> common ground.
- Relay COM -> live side of the non-critical load; load return -> relay NC.
  This keeps the load energized when the relay is de-energized (fail-safe).
- The backend IP for the physical device is configured by editing
  BACKEND_HOST in sketch.ino (use your LAN IP instead of
  host.wokwi.internal, which only exists inside Wokwi).

## Safety

1. Only a licensed electrician may terminate the 380 V side of the panel.
2. Every voltage input runs through a fuse and a rated terminal block.
3. Keep the 380 V zone physically separated from the 5 V logic zone in the
   enclosure.
4. The ESP32 and relay module are not insulation by themselves; always use
   the optocoupler relay module as the isolation boundary.
5. Verify the CT is clamped around exactly one conductor (not the pair), or
   the reading cancels to zero.