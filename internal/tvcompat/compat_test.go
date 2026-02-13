package tvcompat

import (
	"testing"
)

func TestCompatFromVcodec(t *testing.T) {
	tests := []struct {
		name       string
		vcodec     string
		wantCompat string
	}{
		{"H.264 avc1", "avc1.64001f", TvCompatYellow},
		{"H.264 simple", "h264", TvCompatYellow},
		{"H.264 uppercase", "H264", TvCompatYellow},
		{"AVC uppercase", "AVC1.64001f", TvCompatYellow},
		{"VP9", "vp9", TvCompatRed},
		{"VP9 profile", "vp9.0", TvCompatRed},
		{"AV1 short", "av1", TvCompatRed},
		{"AV01 long", "av01.0.08M.08", TvCompatRed},
		{"AV01 simple", "av01", TvCompatRed},
		{"Empty string", "", ""},
		{"None codec", "none", ""},
		{"Unknown codec", "hevc", ""},
		{"H265 unknown", "h265", ""},
		{"Whitespace only", "   ", ""},
		{"VP9 with spaces", "  vp9  ", TvCompatRed},
		{"H264 with spaces", " h264 ", TvCompatYellow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompatFromVcodec(tt.vcodec)
			if got != tt.wantCompat {
				t.Errorf("CompatFromVcodec(%q) = %q, want %q", tt.vcodec, got, tt.wantCompat)
			}
		})
	}
}

func TestFormatH264Level(t *testing.T) {
	tests := []struct {
		level    int
		expected string
	}{
		{41, "4.1"},
		{40, "4"},
		{50, "5"},
		{51, "5.1"},
		{30, "3"},
		{31, "3.1"},
		{42, "4.2"},
		{10, "1"},
		{0, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatH264Level(tt.level)
			if got != tt.expected {
				t.Errorf("formatH264Level(%d) = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestParseAndFormatH264Level_Roundtrip(t *testing.T) {
	levels := []string{"3.0", "3.1", "4.0", "4.1", "4.2", "5.0", "5.1"}
	for _, l := range levels {
		parsed := ParseH264Level(l)
		if parsed == 0 {
			t.Errorf("ParseH264Level(%q) = 0, expected non-zero", l)
			continue
		}
		formatted := formatH264Level(parsed)
		reparsed := ParseH264Level(formatted)
		if reparsed != parsed {
			t.Errorf("Roundtrip failed for %q: parsed=%d, formatted=%q, reparsed=%d", l, parsed, formatted, reparsed)
		}
	}
}
