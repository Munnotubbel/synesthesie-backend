# Audio Pipeline – Lern-Dokumentation

> **Ziel:** Nachvollziehbar erklären wie ein hochgeladenes Audio (FLAC, MP3, WAV) bei Synesthesie verarbeitet, gespeichert und gestreamt wird.

---

## 1. Grundbegriffe

### Was ist eine Audio-Datei?
Eine Audiodatei enthält Tondaten in zwei Schichten:
- **Container-Format** (z.B. `.mp3`, `.flac`, `.m4a`): Die "Verpackung". Enthält Metadaten (Titel, Bitrate, Dauer) und den eigentlichen Ton-Datenstrom.
- **Codec** (z.B. `MP3`, `FLAC`, `AAC`, `Opus`): Der Algorithmus, mit dem der Ton komprimiert (verkleinert) wurde.

| Format | Codec | Verlustfrei? | Typische Größe (3m38s) |
|--------|-------|-------------|----------------------|
| `.flac` | FLAC | ✅ Ja (Lossless) | ~35–80 MB |
| `.mp3` | MP3 | ❌ Nein (Lossy) | ~8–15 MB |
| `.wav` | PCM | ✅ Ja (Uncompressed) | ~100–200 MB |
| `.aac` / `.m4a` | AAC | ❌ Nein (Lossy) | ~8–15 MB |

### Was ist PCM?
PCM (Pulse Code Modulation) ist rohes, unkomprimiertes Audio – exakt so wie die Schallwelle als Zahlenfolge. Eine 3:38 FLAC-Datei enthält intern ~19 Millionen Samples pro Kanal (bei 44.100 Hz Samplerate × 218 Sekunden). Alle weiteren Verarbeitungsschritte starten mit PCM.

---

## 2. Upload & Verarbeitung (Backend)

Wenn ein Admin eine Datei hochlädt, durchläuft sie folgende Schritte (alle in `internal/pkg/audio/processor.go` und `internal/services/music_service.go`):

```
Admin-Client
    │ POST /api/v1/admin/music-sets/:id/upload
    ▼
music_handler.go  → Liest die Datei als []byte
    │
    ▼
music_service.go  → Ruft audio.ProcessAudio() auf
    │
    ├── 1. Schreibt die Datei temporär auf Disk (z.B. /tmp/audio-process-123/input.flac)
    ├── 2. FFprobe  → Exakte Dauer extrahieren
    ├── 3. FFmpeg   → 1000 Hz Mono PCM → 800 Peak-Datenpunkte
    ├── 4. FFmpeg   → HLS Segmente erzeugen (.m3u8 + .ts Chunks)
    │
    ▼
music_service.go  → Alles hochladen:
    ├── music/UUID/master.m3u8        (Playlist für den Stream)
    ├── music/UUID/chunk_000.ts       (Erster 10-Sekunden-Block)
    ├── music/UUID/chunk_001.ts       ...
    ├── music/UUID/chunk_N.ts
    └── music/UUID/source.mp3         (Original-Datei, für Download)
    │
    ▼
Datenbank (PostgreSQL)
    └── music_sets:
        ├── asset_id       → zeigt auf music/UUID/master.m3u8
        ├── source_asset_id → zeigt auf music/UUID/source.mp3
        ├── duration        → Sekunden (von FFprobe)
        └── peak_data       → Waveform-Array mit 800 Float-Werten [0.0 - 1.0]
```

---

## 3. FFmpeg im Detail

FFmpeg ist das Schweizer Taschenmesser der Audio-/Videobearbeitung. Wir rufen es 3x als Subprocess auf:

### 3a. Dauer-Extraktion via FFprobe
```bash
ffprobe -v error \
        -show_entries format=duration \
        -of default=noprint_wrappers=1:nokey=1 \
        input.flac
# Ausgabe: 218.320000
```
FFprobe liest nur den Header der Datei aus — das dauert Millisekunden und lädt nicht die gesamte Audiodatei.

### 3b. Waveform-Peaks via FFmpeg
```bash
ffmpeg -i input.flac \
       -f f32le  \    # Ausgabe als rohe 32-Bit Float-Zahlen
       -ac 1     \    # Mono (1 Kanal)
       -ar 1000  \    # 1000 Samples pro Sekunde
       pipe:1         # In Go's stdin pipen, nicht auf Disk
```
**Warum 1000 Hz?**
Bei 1000 Hz Samplerate gilt: FFmpeg filtert alles über 500 Hz (Nyquist-Theorem) heraus. Das ist fein für Visualisierung: Wir brauchen kein vollständiges Audiospektrum, nur die Amplitudenhüllkurve (wie laut ist's wann).
Bei 100 Hz (vorher) lag die Grenze bei 50 Hz — das filterte alles außer subtiefem Bass heraus → flache Waveform.

**Peak-Extraktion in Go:**
Die 1000 × Duration Samples werden in 800 Bins aufgeteilt. In jedem Bin: absolutes Maximum = Peak. Das ergibt **800 normalisierte Float-Werte** zwischen 0.0 (lautlos) und 1.0 (lautester Moment).

### 3c. HLS-Generierung via FFmpeg

**Fall A: Eingabe ist MP3**
```bash
ffmpeg -i input.mp3 \
       -c:a copy       \    # Audio-Stream direkt 1:1 kopieren (kein Re-Encoding!)
       -f hls          \    # HLS-Container
       -hls_time 10    \    # Jeder Chunk = 10 Sekunden
       -hls_list_size 0 \   # Alle Chunks in der Playlist (auch für VOD)
       -hls_segment_filename /tmp/.../chunk_%03d.ts \
       /tmp/.../master.m3u8
```
`-c:a copy` = **Stream Copy** = die MP3-Daten werden **nicht dekodiert, nicht re-encodiert**, sondern bitgenau in MPEG-TS Container gepackt. **Kein Qualitätsverlust, keine Artefakte.**

**Fall B: Eingabe ist FLAC/WAV/OGG**
```bash
ffmpeg -i input.flac \
       -c:a libmp3lame \    # Transcode zu MP3 (hochwertigster freier MP3-Encoder)
       -V 0            \    # Höchste VBR-Qualität (~245 kbps, klanglich transparent)
       ...
```
`-V 0` = Variable Bitrate, höchste Einstellung. Besser als "320k CBR" weil VBR dem Codec erlaubt, bei komplexen Momenten mehr Bits zu verwenden.

---

## 4. Was sind HLS und MPEG-TS?

### HLS (HTTP Live Streaming)
HLS wurde von Apple entwickelt und ist heute der Standard für Video/Audio-Streaming im Web. Statt einer riesigen Datei gibt es:

1. **Playlist-Datei (`.m3u8`)** – Eine Text-Datei, die dem Player sagt: "Die ersten 10 Sekunden sind in chunk_000.ts, die nächsten in chunk_001.ts, etc."
2. **Segmente (`.ts`)** – Kleine Häppchen-Dateien (bei uns je 10 Sekunden).

```
# Beispiel master.m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0

#EXTINF:10.000000,
chunk_000.ts
#EXTINF:10.000000,
chunk_001.ts
#EXTINF:8.320000,
chunk_021.ts
#EXT-X-ENDLIST
```

**Vorteile von HLS:**
- Ein User kann bei 1:44 einsteigen → der Player lädt nur `chunk_010.ts` statt alles von vorne
- Kein großer Server-Request: 10s-Häppchen statt 150MB am Stück
- Browser kann immer nur die nächsten 3 Chunks puffern → kein RAM-Overflow
- Seek (Vor-/Zurückspulen) funktioniert sauber

### MPEG-TS (`.ts`) Container
MPEG-TS = MPEG Transport Stream. Lange vor Netflix & Spotify entwickelt für TV-Übertragungen. Ein `.ts` Container kann MP3, AAC, H.264 Video usw. "transportieren". Bei uns enthält jeder `.ts` Chunk pure MP3-Audiodaten.

---

## 5. S3 Bucket Struktur

Nach dem Upload sieht der S3 Bucket so aus:

```
media-audio-bucket/
└── music/
    └── {UUID}/
        ├── master.m3u8        ← Playlist (wenige KB)
        ├── chunk_000.ts       ← 10s Audio (ca. 300–400 KB)
        ├── chunk_001.ts
        ├── ...
        ├── chunk_021.ts       ← Letzter Chunk
        └── source.mp3         ← Original-Datei (für Download)
```

---

## 6. Streaming – Request/Response Flow

### 6a. Abspielen (HLS)

```
Browser (hls.js)
    │
    │ GET /api/v1/admin/music-sets/:id/stream/master.m3u8?token=...
    ▼
Gin Router → music_handler.go → streamMusicSet()
    │  filepath param = "/master.m3u8"
    │  baseS3Key = "music/UUID"
    │  targetS3Key = "music/UUID/master.m3u8"
    │
    ▼
S3Service.StreamMedia() → Streamt m3u8-Text direkt als HTTP Response
    │
    ▼
Browser / hls.js liest die m3u8

    │ (für jeden Chunk)
    │ GET /api/v1/admin/music-sets/:id/stream/chunk_000.ts?token=...
    ▼
Gin Router → streamMusicSet()
    │  targetS3Key = "music/UUID/chunk_000.ts"
    ▼
S3Service.StreamMedia() → Streamt die .ts Binärdaten
    │
    ▼
Browser / hls.js dekodiert MP3 aus .ts → Ton spielt ab
```

### 6b. Herunterladen (Original-Datei)

```
Browser
    │ GET /api/v1/admin/music-sets/:id/stream?download=true&token=...
    ▼
streamMusicSet() erkennt download=true
    │  Überspringt HLS-Logik komplett
    │  Lädt source_asset_id aus DB
    │  targetS3Key = "music/UUID/source.mp3"
    │
    ▼
S3 → Streamt die Original-MP3 mit:
    Content-Disposition: attachment; filename="Let it go.mp3"
```

---

## 7. Waveform-Visualisierung

Die Waveform-Daten (800 Floats) sind in der Datenbank im `peak_data` Feld als JSONB gespeichert.

Wenn die API ein MusicSet zurückgibt, sind die Peaks dabei:
```json
{
  "id": "...",
  "title": "Let it Go",
  "duration": 218,
  "peak_data": [0.012, 0.045, 0.089, ...]
}
```

Das Frontend gibt diese 800 Werte direkt an WaveSurfer weiter:
```tsx
WaveSurfer.create({
  media: audioElement,
  peaks: [set.peak_data],
  duration: set.duration,
});
```

WaveSurfer rendert dann sofort die Waveform-Grafik — **ohne die Audiodatei selbst analysieren zu müssen**. Das spart massiv RAM und Zeit.

---

## 8. Qualitätsstufen im Vergleich

| Szenario | Beschreibung | Qualitätsverlust? |
|----------|-------------|-------------------|
| MP3 hochladen + abspielen | Stream Copy → kein Re-Encoding | ✅ Keiner |
| FLAC hochladen + abspielen | FLAC → `libmp3lame -V 0` | Minimal, klanglich transparent |
| WAV hochladen + abspielen | WAV → `libmp3lame -V 0` | Minimal, klanglich transparent |
| Download | Immer die Original-Datei | ✅ Keiner |

### Warum nicht FLAC als HLS-Stream?
FLAC in HLS würde funktionieren (via fMP4-Container), aber:
- Jeder 10s FLAC-Chunk wäre ~2 MB statt ~300 KB
- 4h Techno-Set = 3.000 Chunks × 2 MB = 6 GB Datentransfer beim Abspielen
- `libmp3lame -V 0` ist bei ~245 kbps für praktisch jeden Hörer nicht von Lossless zu unterscheiden

### Warum nicht Opus oder AAC?
- **Opus** wäre klanglich besser als MP3 — aber Alpine Linux FFmpeg enthält kein `libopus-encoder` standardmäßig
- **AAC** über Apples `libfdk_aac` wäre ausgezeichnet — aber ebenfalls nicht in Alpine (Lizenzproblem). Der eingebaute `aac`-Encoder ist hörbar schlechter als MP3 bei gleicher Bitrate

---

## 9. Dateien im Überblick

| Datei | Zuständigkeit |
|-------|--------------|
| `internal/pkg/audio/processor.go` | FFprobe/FFmpeg Wrapper: Dauer, Peaks, HLS-Segmente |
| `internal/services/music_service.go` | Upload-Flow: Aufruf Processor → S3 Upload → DB Update |
| `internal/handlers/music_handler.go` | HTTP Handler: Stream / Download Endpunkte |
| `internal/models/music_set.go` | Datenbank-Modell (PeakData, Duration, AssetID, SourceAssetID) |
| `cmd/api/main.go` | Gin Router: `/stream` und `/stream/*filepath` Routen |
