package tvcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
)

type timingAssessment int

const (
	timingSupported timingAssessment = iota
	timingAboveLimit
	timingWindowSeconds = 3.0
	// Timestamp quantization and rounding can slightly shorten a nominal 30fps interval.
	timingFPSMargin = 0.5
)

// samplePacketTiming examines bounded windows near the beginning, middle and end.
// It detects observed pacing mismatches without treating nominal r_frame_rate as
// actual content FPS. It cannot establish behavior of every packet in a long file.
func samplePacketTiming(ctx context.Context, path string, duration float64) (timingAssessment, error) {
	intervals := "0%+3"
	if duration > timingWindowSeconds*3 {
		intervals = fmt.Sprintf("0%%+3,%.3f%%+3,%.3f%%+3", duration/2, duration-timingWindowSeconds)
	}
	// #nosec G204 -- fixed executable/options, local path and numeric intervals are individual arguments.
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-read_intervals", intervals, "-show_packets", "-show_entries", "packet=pts_time", "-of", "json", path).Output()
	if err != nil {
		return timingSupported, inconclusiveTiming()
	}
	var data struct {
		Packets []struct {
			PTSTime string `json:"pts_time"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return timingSupported, inconclusiveTiming()
	}
	times := make([]float64, 0, len(data.Packets))
	for _, packet := range data.Packets {
		pts, parseErr := strconv.ParseFloat(packet.PTSTime, 64)
		if parseErr != nil || math.IsNaN(pts) || math.IsInf(pts, 0) {
			return timingSupported, inconclusiveTiming()
		}
		times = append(times, pts)
	}
	return assessPacketTimes(times)
}

func assessPacketTimes(times []float64) (timingAssessment, error) {
	sort.Float64s(times)
	samples := 0
	for i := 1; i < len(times); i++ {
		delta := times[i] - times[i-1]
		if delta == 0 {
			continue
		} // repeated seek windows may include the same packet
		if delta < 1/(maxTVFrameRate+timingFPSMargin) {
			return timingAboveLimit, nil
		}
		if delta <= timingWindowSeconds {
			samples++
		}
	}
	if samples < 2 {
		return timingSupported, inconclusiveTiming()
	}
	return timingSupported, nil
}
