package tvcompat

import (
	"testing"
)

func TestParseH264Level(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"4.1", "4.1", 41},
		{"4.0", "4.0", 40},
		{"5.0", "5.0", 50},
		{"5.1", "5.1", 51},
		{"3.0", "3.0", 30},
		{"4", "4", 40},
		{"5", "5", 50},
		{"empty", "", 0},
		{"with spaces", "  4.1  ", 41},
		{"invalid major", "x", 0},
		{"invalid major number", "10", 0},
		{"negative", "-1", 0},
		{"4.2", "4.2", 42},
		{"0", "0", 0},
		{"0.1", "0.1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseH264Level(tt.input)
			if got != tt.want {
				t.Errorf("ParseH264Level(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestTvCompatConstants(t *testing.T) {
	if TvCompatGreen != "green" {
		t.Errorf("TvCompatGreen = %q, want green", TvCompatGreen)
	}
	if TvCompatYellow != "yellow" {
		t.Errorf("TvCompatYellow = %q, want yellow", TvCompatYellow)
	}
	if TvCompatRed != "red" {
		t.Errorf("TvCompatRed = %q, want red", TvCompatRed)
	}
}
