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
		{"HEVC", "hevc", TvCompatRed},
		{"H265", "h265", TvCompatRed},
		{"H265 profile", "H265.Main10", TvCompatRed},
		{"Empty string", "", ""},
		{"None codec", "none", ""},
		{"Unknown codec", "opus", ""},
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
