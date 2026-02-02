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

func TestCompatFromTorrentFileNames(t *testing.T) {
	tests := []struct {
		name       string
		paths      []string
		wantCompat string
	}{
		{"mkv first", []string{"Movie.mkv"}, TvCompatGreen},
		{"mp4 first", []string{"Movie.mp4"}, TvCompatGreen},
		{"mov first", []string{"Movie.mov"}, TvCompatGreen},
		{"m4v first", []string{"Movie.m4v"}, TvCompatGreen},
		{"avi first", []string{"Movie.avi"}, TvCompatYellow},
		{"webm first", []string{"Movie.webm"}, TvCompatRed},
		{"poster then mkv", []string{"poster.jpg", "Movie.mkv"}, TvCompatGreen},
		{"srt then mkv", []string{"sub.srt", "Movie.mkv"}, TvCompatGreen},
		{"no video", []string{"readme.txt", "poster.jpg"}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompatFromTorrentFileNames(tt.paths)
			if got != tt.wantCompat {
				t.Errorf("CompatFromTorrentFileNames(%v) = %q, want %q", tt.paths, got, tt.wantCompat)
			}
		})
	}
}

func TestIsVideoFilePath(t *testing.T) {
	video := []string{"a.mkv", "a.mp4", "a.avi", "a.mov", "a.webm", "a.m4v", "dir/v.MKV"}
	nonVideo := []string{"a.jpg", "a.srt", "a.nfo", "a.txt", ""}
	for _, p := range video {
		if !IsVideoFilePath(p) {
			t.Errorf("IsVideoFilePath(%q) = false, want true", p)
		}
	}
	for _, p := range nonVideo {
		if IsVideoFilePath(p) {
			t.Errorf("IsVideoFilePath(%q) = true, want false", p)
		}
	}
}
