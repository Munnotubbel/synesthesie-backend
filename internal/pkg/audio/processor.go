package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessResult contains all data extracted and generated from an audio file
type ProcessResult struct {
	Duration int
	PeakData []float32
	HLSFiles map[string][]byte // Filename -> Data (master.m3u8, chunk_000.ts, etc)
}

// ProcessAudio takes raw audio bytes, writes to a temp file, runs FFmpeg to extract
// duration, 150-point peak array, and generates HLS segments.
func ProcessAudio(ctx context.Context, audioData []byte, ext string) (*ProcessResult, error) {
	// Create a temporary directory for processing
	tempDir, err := os.MkdirTemp("", "audio-process-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // cleanup after we are done

	inFile := filepath.Join(tempDir, "input"+ext)
	if err := os.WriteFile(inFile, audioData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}

	result := &ProcessResult{
		HLSFiles: make(map[string][]byte),
	}

	// 1. Get Duration
	log.Printf("[HLS] Extracting duration for %s", inFile)
	durationStr, err := runCommand(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inFile)
	if err != nil {
		log.Printf("[HLS] Warning: failed to extract duration: %v", err)
	} else {
		// Parse float and round to int
		if d, err := strconv.ParseFloat(strings.TrimSpace(durationStr), 64); err == nil {
			result.Duration = int(math.Round(d))
		}
	}

	// 2. Extract 1000 point Waveform Peak Data
	// Convert to 1000Hz mono raw f32le.
	// At 100Hz, ffmpeg's lowpass filter (Nyquist=50Hz) destroys all transients,
	// making the waveform look flat. At 1000Hz we capture the full amplitude
	// envelope accurately. For a 4h track: ~57MB in memory, which is acceptable.
	log.Printf("[HLS] Extracting waveform peaks for %s", inFile)
	pcmData, err := runCommandBytes(ctx, "ffmpeg",
		"-i", inFile,
		"-f", "f32le",
		"-ac", "1",
		"-ar", "1000", // 1000 samples per second
		"pipe:1")
	if err != nil {
		log.Printf("[HLS] Warning: failed to extract raw pcm data for peaks: %v", err)
	} else {
		result.PeakData = calculatePeaks(pcmData, 1000)
		log.Printf("[HLS] Generated %d peak data points", len(result.PeakData))
	}

	// 3. Detect the input audio codec to decide whether to copy or re-encode.
	// For MP3 sources: stream-copy = bit-perfect, zero quality loss, no bass distortion.
	// For FLAC/WAV/OGG: transcode to libmp3lame at highest VBR quality.
	log.Printf("[HLS] Generating HLS segments for %s", inFile)
	hlsOutFile := filepath.Join(tempDir, "master.m3u8")

	inputCodec, _ := runCommand(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inFile)
	inputCodec = strings.TrimSpace(inputCodec)
	log.Printf("[HLS] Input audio codec detected: %s", inputCodec)

	hlsArgs := []string{"-i", inFile}
	if inputCodec == "mp3" {
		// MP3 source: stream-copy into MPEG-TS segments — bit-perfect, no artifacts
		log.Printf("[HLS] Using stream copy for MP3 input (zero quality loss)")
		hlsArgs = append(hlsArgs, "-c:a", "copy")
	} else {
		// FLAC/WAV/OGG/etc: transcode with highest quality libmp3lame VBR mode
		// -V 0 = highest VBR quality (~245kbps average, transparent)
		log.Printf("[HLS] Transcoding %s → libmp3lame VBR 0", inputCodec)
		hlsArgs = append(hlsArgs, "-c:a", "libmp3lame", "-V", "0")
	}
	hlsArgs = append(hlsArgs,
		"-f", "hls",
		"-hls_time", "10", // 10 second chunks
		"-hls_list_size", "0", // keep all segments in playlist
		"-hls_segment_filename", filepath.Join(tempDir, "chunk_%03d.ts"),
		hlsOutFile)

	_, err = runCommand(ctx, "ffmpeg", hlsArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate hls: %w", err)
	}

	// 4. Read all generated files back into memory map
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp dir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".m3u8") || strings.HasSuffix(name, ".ts") {
			data, err := os.ReadFile(filepath.Join(tempDir, name))
			if err != nil {
				return nil, fmt.Errorf("failed to read generated file %s: %w", name, err)
			}
			result.HLSFiles[name] = data
		}
	}

	log.Printf("[HLS] Successfully generated %d HLS files", len(result.HLSFiles))
	return result, nil
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	out, err := runCommandBytes(ctx, name, args...)
	return string(out), err
}

func runCommandBytes(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Add a 60 minute timeout so we don't hang forever on weird audio,
	// but allow enough time to transcode 4-hour techno sets.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Just log stderr and return the error.
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// calculatePeaks divides raw 32-bit float PCM data into `count` bins,
// extracting the absolute maximum value in each bin, and smoothing.
func calculatePeaks(pcmData []byte, count int) []float32 {
	if len(pcmData) == 0 || count <= 0 {
		return make([]float32, count)
	}

	// Interpret pcmData as []float32
	samplesCount := len(pcmData) / 4
	samples := make([]float32, samplesCount)
	for i := 0; i < samplesCount; i++ {
		bits := binary.LittleEndian.Uint32(pcmData[i*4 : i*4+4])
		samples[i] = math.Float32frombits(bits)
	}

	// Ensure we have at least 'count' samples to sample correctly, else pad logic
	if samplesCount < count {
		// Pad with existing logic
	}

	peaks := make([]float32, count)
	binSize := float64(samplesCount) / float64(count)

	var globalMax float32 = 0.0

	for i := 0; i < count; i++ {
		start := int(float64(i) * binSize)
		end := int(float64(i+1) * binSize)
		if end > samplesCount {
			end = samplesCount
		}

		// Peak detection: use the maximum absolute value in this bin.
		// This is how professional players like GNOME Audio render waveforms:
		// each bar reflects the loudest instant, not the average energy (RMS).
		// This preserves transients like snares, hi-hats, and attack peaks.
		var maxVal float32 = 0.0
		for j := start; j < end; j++ {
			val := samples[j]
			if val < 0 {
				val = -val // abs()
			}
			if val > maxVal {
				maxVal = val
			}
		}

		peaks[i] = maxVal
		if maxVal > globalMax {
			globalMax = maxVal
		}
	}

	// Normalize to 0.0 - 1.0
	if globalMax > 0 {
		for i := range peaks {
			peaks[i] = peaks[i] / globalMax
		}
	}

	return peaks
}
