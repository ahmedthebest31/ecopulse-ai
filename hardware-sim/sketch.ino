// EcoPulse AI - ESP32 factory energy monitor firmware for the Wokwi
// simulator. It plays the role of the physical factory BOM: the on-screen
// potentiometer stands in for the SCT-013 current transformer, and the
// green/red LEDs, buzzer and relay module visualize the backend's actuation
// commands.
//
// Flow:
//   1. Sample the ADC (potentiometer = CT) and convert it to a per-phase
//      current, then to three-phase power and an accumulated kWh counter.
//   2. POST the reading to the Go backend every REPORT_INTERVAL_MS.
//   3. Parse the actuation reply (alert level, tariff tier, peak hour) and
//      drive the LEDs, buzzer and relay without ever blocking the loop.
//
// WiFi reaches the backend through the Wokwi Private IoT Gateway at
// host.wokwi.internal (this also works in the VS Code extension).

#include <WiFi.h>
#include <HTTPClient.h>
#include <time.h>

// ---------------------------------------------------------------------------
// Wiring / configuration
// ---------------------------------------------------------------------------

const int ADC_PIN      = 34;  // potentiometer "SIG" (simulated CT input)
const int LED_OK_PIN   = 2;   // green "OK" LED
const int LED_WARN_PIN = 4;   // red "WARN" LED
const int BUZZER_PIN   = 17;  // buzzer positive pin (pin 1 is the negative)
const int RELAY_PIN    = 32;  // relay module IN pin (active-high npn module)

const char* WIFI_SSID     = "Wokwi-GUEST";   // simulated WiFi network
const char* WIFI_PASS     = "";              // open network, no password
const uint8_t WIFI_CHANNEL = 6;              // fixed channel, skips the scan
const char* BACKEND_HOST  = "host.wokwi.internal";
const uint16_t BACKEND_PORT = 8080;
const char* DEVICE_ID     = "esp32-factory-001";

// Sensing / power model. The factory is a 380 V line-to-line three-phase
// service; the CT measures one phase and MAX_AMPS sets the full-scale reading
// of the simulated ADC (12-bit, 0..4095).
const unsigned long REPORT_INTERVAL_MS = 5000UL;
const unsigned long SAMPLE_INTERVAL_MS = 250UL;
const unsigned long WIFI_RETRY_MS      = 3000UL;
const int MOVING_AVG_N    = 16;
const float ADC_MAX       = 4095.0F;
const float MAX_AMPS      = 200.0F;  // CT full-scale, per phase
const float VOLTAGE_RMS   = 380.0F;  // line-to-line, three phase
const float POWER_FACTOR  = 0.9F;
const float SQRT3         = 1.7320508F;
const unsigned long RESTORE_DELAY_MS = 60000UL;  // re-arm lockout after a shed
const long   MIN_VALID_EPOCH = 1700000000L;      // old enough to prove NTP sync
const long   BOOT_EPOCH      = 1710000000L;      // uptime fallback base date

// ---------------------------------------------------------------------------
// Runtime state
// ---------------------------------------------------------------------------

enum AlertLevel { ALERT_NORMAL = 0, ALERT_WARNING, ALERT_CRITICAL };

const char* kAlertNames[] = { "NORMAL", "WARNING", "CRITICAL" };

// Moving average over the raw ADC samples.
int   adcSamples[MOVING_AVG_N];
int   adcIdx = 0;
long  adcSum = 0;

float currentAmps = 0.0F;
float kwNow       = 0.0F;
float energyKwh   = 0.0F;

bool  wifiConnected = false;
bool  lastPostOK    = false;
AlertLevel alert    = ALERT_NORMAL;
AlertLevel prevAlert = ALERT_NORMAL;
long  currentTier   = 1;
bool  isPeakHour    = false;
bool  shedRecommended = false;

unsigned long lastSampleMs = 0;
unsigned long lastReportMs = 0;
unsigned long lastWifiTryMs = 0;
unsigned long shedSinceMs  = 0;
bool  relayShed  = false;

String backendURL;

// ---------------------------------------------------------------------------
// Non-blocking beep scheduler
// ---------------------------------------------------------------------------

struct BeepSlot {
  unsigned long startMs;
  unsigned long durationMs;
  bool pending;
};

#define MAX_BEEPS 6
BeepSlot beeps[MAX_BEEPS];
bool beepActive = false;
bool lastBeepActive = false;

void queueBeep(unsigned long startOffsetMs, unsigned long durationMs) {
  for (int i = 0; i < MAX_BEEPS; i++) {
    if (!beeps[i].pending) {
      beeps[i].pending = true;
      beeps[i].startMs = millis() + startOffsetMs;
      beeps[i].durationMs = durationMs;
      return;
    }
  }
}

// Schedules count pulses separated by gapMs, e.g. a triple alarm beep:
// queueBeeps(3, 120, 100).
void queueBeeps(int count, unsigned long durationMs, unsigned long gapMs) {
  for (int i = 0; i < count; i++) {
    queueBeep((unsigned long)i * (durationMs + gapMs), durationMs);
  }
}

// Turns the buzzer on while at least one slot is within its pulse window and
// removes pulses that already finished.
void serviceBeeps(unsigned long nowMs) {
  bool anyActive = false;
  for (int i = 0; i < MAX_BEEPS; i++) {
    if (!beeps[i].pending) {
      continue;
    }
    unsigned long endMs = beeps[i].startMs + beeps[i].durationMs;
    if (nowMs >= beeps[i].startMs && nowMs < endMs) {
      anyActive = true;
    }
    if (nowMs >= endMs) {
      beeps[i].pending = false;
    }
  }
  beepActive = anyActive;
  if (beepActive != lastBeepActive) {
    if (beepActive) {
      tone(BUZZER_PIN, 1000);
    } else {
      noTone(BUZZER_PIN);
    }
    lastBeepActive = beepActive;
  }
}

// ---------------------------------------------------------------------------
// Tiny JSON response parser (the payload is a known backend contract, so we
// only ever read keys we wrote on the server; no JSON library is needed).
// ---------------------------------------------------------------------------

// findJSONString looks for "key":"value" and copies value into out.
bool findJSONString(const char* json, const char* key, char* out, size_t outSize) {
  char pattern[48];
  snprintf(pattern, sizeof(pattern), "\"%s\":", key);
  const char* p = strstr(json, pattern);
  if (!p) {
    return false;
  }
  p += strlen(pattern);
  while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') {
    p++;
  }
  if (*p != '"') {
    return false;
  }
  p++;
  size_t i = 0;
  while (*p && *p != '"' && i + 1 < outSize) {
    out[i++] = *p;
    p++;
  }
  out[i] = '\0';
  return i > 0;
}

// findJSONLong looks for "key":123 and writes the integer into out.
bool findJSONLong(const char* json, const char* key, long* out) {
  char pattern[48];
  snprintf(pattern, sizeof(pattern), "\"%s\":", key);
  const char* p = strstr(json, pattern);
  if (!p) {
    return false;
  }
  p += strlen(pattern);
  while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') {
    p++;
  }
  char buffer[24];
  size_t i = 0;
  if (*p == '-') {
    buffer[i++] = *p;
    p++;
  }
  while ((*p >= '0' && *p <= '9') && i + 1 < sizeof(buffer)) {
    buffer[i++] = *p;
    p++;
  }
  buffer[i] = '\0';
  if (i == 0) {
    return false;
  }
  *out = atol(buffer);
  return true;
}

AlertLevel alertFromName(const char* name) {
  // The backend always emits uppercase level names; strict compare is enough.
  if (strcmp(name, "WARNING") == 0) {
    return ALERT_WARNING;
  }
  if (strcmp(name, "CRITICAL") == 0) {
    return ALERT_CRITICAL;
  }
  return ALERT_NORMAL;
}

// ---------------------------------------------------------------------------
// Timestamp
// ---------------------------------------------------------------------------

// iso8601Now writes an RFC3339 UTC timestamp. It prefers NTP-derived wall
// time and falls back to an uptime-based time once NTP never answered.
void iso8601Now(char* out, size_t outSize) {
  time_t now = time(nullptr);
  struct tm tmv;
  bool ok = now >= MIN_VALID_EPOCH && gmtime_r(&now, &tmv) != nullptr;
  if (!ok) {
    now = BOOT_EPOCH + (long)(millis() / 1000UL);
    gmtime_r(&now, &tmv);
  }
  char datePart[24];
  strftime(datePart, sizeof(datePart), "%Y-%m-%dT%H:%M:%S", &tmv);
  snprintf(out, outSize, "%sZ", datePart);
}

// ---------------------------------------------------------------------------
// Sampling
// ---------------------------------------------------------------------------

// sampleReading integrates one ADC sample into the moving average and rolls
// the power and energy numbers forward.
void sampleReading(unsigned long nowMs) {
  int raw = analogRead(ADC_PIN);
  adcSum -= adcSamples[adcIdx];
  adcSamples[adcIdx] = raw;
  adcSum += raw;
  adcIdx = (adcIdx + 1) % MOVING_AVG_N;

  float avg = (float)adcSum / (float)MOVING_AVG_N;
  float amps = (avg / ADC_MAX) * MAX_AMPS;
  if (amps < 0.0F) {
    amps = 0.0F;
  }
  if (amps > MAX_AMPS) {
    amps = MAX_AMPS;
  }
  currentAmps = amps;
  kwNow = SQRT3 * VOLTAGE_RMS * amps * POWER_FACTOR / 1000.0F;

  float dtSeconds = (float)(nowMs - lastSampleMs) / 1000.0F;
  if (dtSeconds > 0.0F && dtSeconds < 3600.0F) {
    energyKwh += kwNow * dtSeconds / 3600.0F;
  }
  lastSampleMs = nowMs;
}

// ---------------------------------------------------------------------------
// Telemetry
// ---------------------------------------------------------------------------

// postReading sends the current reading and updates the actuation state from
// the backend reply. Returns true only when the server answered 200.
bool postReading() {
  char timestamp[24];
  iso8601Now(timestamp, sizeof(timestamp));

  char body[256];
  snprintf(body, sizeof(body),
           "{\"device_id\":\"%s\",\"timestamp\":\"%s\",\"amperage\":%.2f,"
           "\"voltage\":%.0f,\"kilowatts\":%.2f,\"accumulated_kwh\":%.3f}",
           DEVICE_ID, timestamp, currentAmps, VOLTAGE_RMS, kwNow, energyKwh);

  HTTPClient http;
  http.begin(backendURL);
  http.setTimeout(4000);
  http.setUserAgent("ecopulse-esp32");
  http.addHeader("Content-Type", "application/json");
  Serial.printf("POST %s\n", body);

  int code = http.POST(body);
  if (code != HTTP_CODE_OK) {
    Serial.printf("POST failed: HTTP %d\n", code);
    http.end();
    return false;
  }

  String payload = http.getString();
  http.end();

  char levelName[16] = "NORMAL";
  long tier = 1;
  long peak = 0;
  long shed = 0;
  if (findJSONString(payload.c_str(), "alert_level", levelName, sizeof(levelName))) {
    alert = alertFromName(levelName);
  }
  if (findJSONLong(payload.c_str(), "current_tier", &tier)) {
    currentTier = tier;
  }
  if (findJSONLong(payload.c_str(), "is_peak_hour", &peak)) {
    isPeakHour = peak != 0;
  }
  if (findJSONLong(payload.c_str(), "load_shed_recommended", &shed)) {
    shedRecommended = shed != 0;
  }

  Serial.printf("<- alert=%s tier=%ld peak=%d shed=%d\n",
                kAlertNames[alert], currentTier, (int)isPeakHour, shedRecommended);
  return true;
}

// ---------------------------------------------------------------------------
// Actuation
// ---------------------------------------------------------------------------

// ensureWiFi re-attempts the association in a throttled, non-blocking way.
void ensureWiFi(unsigned long nowMs) {
  if (WiFi.status() == WL_CONNECTED) {
    if (!wifiConnected) {
      Serial.printf("WiFi connected: %s\n", WiFi.localIP().toString().c_str());
      wifiConnected = true;
    }
    lastWifiTryMs = nowMs;
    return;
  }
  wifiConnected = false;
  if (nowMs - lastWifiTryMs >= WIFI_RETRY_MS) {
    Serial.println("WiFi lost, reconnecting...");
    WiFi.begin(WIFI_SSID, WIFI_PASS, WIFI_CHANNEL);
    lastWifiTryMs = nowMs;
  }
}

// updateActuators drives the LEDs, buzzer and relay from the current alert
// level. The relay uses the module's active-high (npn) contact behavior:
//    IN high / disconnected -> COM-NC closed -> load ON (normal)
//    IN low                 -> COM opens       -> load OFF (shed, CRITICAL)
void updateActuators(unsigned long nowMs) {
  serviceBeeps(nowMs);

  if (alert != prevAlert) {
    if (alert == ALERT_WARNING) {
      queueBeeps(1, 150, 0);  // single short beep when entering WARNING
    }
    prevAlert = alert;
  }

  switch (alert) {
    case ALERT_CRITICAL:
      if (!relayShed) {
        relayShed = true;
        shedSinceMs = nowMs;
        queueBeeps(3, 120, 100);  // triple alarm on shed
        Serial.println("ACTUATE: load shed (relay OFF)");
      }
      break;
    case ALERT_WARNING:
      // Leave the relay untouched during WARNING; only warn the operator.
      break;
    case ALERT_NORMAL:
      if (relayShed && nowMs - shedSinceMs >= RESTORE_DELAY_MS) {
        relayShed = false;
        Serial.println("ACTUATE: load restored (relay ON)");
      }
      break;
  }

  // Green: 500 ms heartbeat every 2 s while NORMAL and the last POST
  // succeeded; off otherwise. Red: slow blink WARNING, fast blink CRITICAL.
  bool greenWanted = false;
  bool redWanted = false;
  if (alert == ALERT_NORMAL) {
    greenWanted = lastPostOK && (nowMs % 2000UL) < 500UL;
  } else if (alert == ALERT_WARNING) {
    redWanted = (nowMs % 1000UL) < 500UL;
  } else {
    redWanted = (nowMs % 200UL) < 100UL;
  }
  digitalWrite(LED_OK_PIN, greenWanted ? HIGH : LOW);
  digitalWrite(LED_WARN_PIN, redWanted ? HIGH : LOW);
  digitalWrite(RELAY_PIN, relayShed ? LOW : HIGH);
}

// ---------------------------------------------------------------------------
// Arduino lifecycle
// ---------------------------------------------------------------------------

void setup() {
  Serial.begin(115200);
  delay(200);
  Serial.println("EcoPulse AI ESP32 firmware booting...");

  pinMode(LED_OK_PIN, OUTPUT);
  pinMode(LED_WARN_PIN, OUTPUT);
  pinMode(RELAY_PIN, OUTPUT);
  digitalWrite(RELAY_PIN, HIGH);  // non-critical load on until the first verdict

  for (int i = 0; i < MOVING_AVG_N; i++) {
    adcSamples[i] = 0;
  }
  adcSum = 0;

  backendURL = String("http://") + BACKEND_HOST + ":" + BACKEND_PORT +
               "/api/telemetry/iot";

  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASS, WIFI_CHANNEL);
  configTime(0, 0, "pool.ntp.org");  // best-effort; uptime fallback handles it

  lastSampleMs = millis();
  lastReportMs = millis();
  lastWifiTryMs = millis();
}

void loop() {
  unsigned long now = millis();

  if (now - lastSampleMs >= SAMPLE_INTERVAL_MS) {
    sampleReading(now);
  }

  ensureWiFi(now);

  if (now - lastReportMs >= REPORT_INTERVAL_MS) {
    lastReportMs = now;
    if (WiFi.status() == WL_CONNECTED) {
      bool ok = postReading();
      lastPostOK = ok;
      if (!ok) {
        Serial.println("Keeping last actuation state until the backend responds.");
      }
    } else {
      lastPostOK = false;
    }
  }

  updateActuators(now);
}