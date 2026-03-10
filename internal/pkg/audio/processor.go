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

	result := &ProcessResult{}

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

		// Peak Amplitude Calculation (matches WaveSurfer's internal logic)
		var maxPeak float32 = 0.0
		for j := start; j < end; j++ {
			val := float32(math.Abs(float64(samples[j])))
			if val > maxPeak {
				maxPeak = val
			}
		}

		peaks[i] = maxPeak
		if maxPeak > globalMax {
			globalMax = maxPeak
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
