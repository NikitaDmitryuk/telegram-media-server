package tvcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validProbe = `{"streams":[{"codec_type":"video","codec_name":"h264","profile":"Main","level":41,"width":320,"height":180,"pix_fmt":"yuv420p","field_order":"progressive","sample_aspect_ratio":"1:1","r_frame_rate":"30000/1001","avg_frame_rate":"30000/1001"},{"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"sample_rate":"48000"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"1.0"}}`

func validMedia(t *testing.T) *mediaInfo {
	t.Helper()
	var info mediaInfo
	if err := json.Unmarshal([]byte(validProbe), &info); err != nil {
		t.Fatal(err)
	}
	return &info
}

func TestValidateProfile(t *testing.T) {
	cases := []struct {
		name   string
		change func(*mediaInfo)
	}{
		{"audio codec", func(m *mediaInfo) { m.Streams[1].CodecName = "opus" }},
		{"audio profile", func(m *mediaInfo) { m.Streams[1].Profile = "HE-AAC" }},
		{"audio channels", func(m *mediaInfo) { m.Streams[1].Channels = 6 }},
		{"sample rate", func(m *mediaInfo) { m.Streams[1].SampleRate = "96000" }},
		{"unsupported profile", func(m *mediaInfo) { m.Streams[0].Profile = "High 10" }},
		{"high level", func(m *mediaInfo) { m.Streams[0].Level = 51 }},
		{"pixel format", func(m *mediaInfo) { m.Streams[0].PixFmt = "yuv420p10le" }},
		{"fps", func(m *mediaInfo) { m.Streams[0].AvgFrameRate = "60/1" }},

		{"interlaced", func(m *mediaInfo) { m.Streams[0].FieldOrder = "tt" }},
		{"resolution", func(m *mediaInfo) { m.Streams[0].Height = 2160 }},
		{"odd dimensions", func(m *mediaInfo) { m.Streams[0].Width = 319 }},
		{"container", func(m *mediaInfo) { m.Format.FormatName = "matroska,webm" }},
		{"no audio", func(m *mediaInfo) { m.Streams = m.Streams[:1] }},
	}
	if err := validateProfile(validMedia(t), defaultTVHeight, tvLevel); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := validMedia(t)
			c.change(info)
			if validateProfile(info, defaultTVHeight, tvLevel) == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}

func writeTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	// #nosec G703 -- helper paths are generated entirely by these tests under t.TempDir.
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func fakeTools(t *testing.T, ffmpeg string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "ffprobe"), []byte(fakeProbeScript), 0o700)
	writeTestFile(t, filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\n"+ffmpeg), 0o700)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("TV_TEST_PROBE", validProbe)
	t.Setenv("TV_TEST_PACKETS", `{"packets":[{"pts_time":"0"},{"pts_time":"0.033367"},{"pts_time":"0.066733"},{"pts_time":"0.100100"}]}`)
	source := filepath.Join(t.TempDir(), "movie.mp4")
	writeTestFile(t, source, []byte("the original"), 0o600)
	return source
}

const fakeEncode = `for output do :; done
printf '\000\000\000\010ftyp\000\000\000\010moov\000\000\000\010mdat' > "$output"
`

func TestPreparePreservesOriginalAndNeverOverwritesBackup(t *testing.T) {
	source := fakeTools(t, fakeEncode)
	original, _ := os.ReadFile(source)
	for range 2 {
		// Force remux by adding an extra stream only to the source-side metadata.
		forceSourcePreparation(t)
		if err := prepareFile(context.Background(), source, 0, tvLevel); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*", "movie.mp4"))
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups=%v err=%v", backups, err)
	}
	found := false
	for _, p := range backups {
		data, _ := os.ReadFile(p)
		if bytes.Equal(data, original) {
			found = true
		}
	}
	if !found {
		t.Fatal("original backup lost")
	}
	current, _ := os.ReadFile(source)
	if bytes.Equal(current, original) || !hasFastStart(source) {
		t.Fatal("prepared output not published")
	}
	assertNoTemps(t, source)
}

func assertNoTemps(t *testing.T, source string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-*.mp4"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("temporary files remain: %v (%v)", paths, err)
	}
}

func TestPrepareFailuresPreserveSource(t *testing.T) {
	for _, name := range []string{"ffmpeg", "probe", "validation", "cancel", "faststart", "truncated", "container", "backup"} {
		t.Run(name, func(t *testing.T) {
			script := fakeEncode
			if name == "ffmpeg" {
				script = "exit 1\n"
			}
			if name == "faststart" {
				script = "for output do :; done\nprintf bad > \"$output\"\n"
			}
			source := fakeTools(t, script)
			if name == "container" {
				newPath := strings.TrimSuffix(source, ".mp4") + ".mkv"
				if err := os.Rename(source, newPath); err != nil {
					t.Fatal(err)
				}
				source = newPath
			}
			original, _ := os.ReadFile(source)
			if name == "backup" {
				writeTestFile(t, filepath.Join(filepath.Dir(source), ".tvcompat-originals"), []byte("obstruction"), 0o600)
			}
			if name == "probe" {
				t.Setenv("TV_TEST_PROBE", "invalid json")
			}
			if name == "validation" {
				t.Setenv("TV_TEST_PROBE", strings.Replace(validProbe, `"channels":2`, `"channels":6`, 1))
			}
			if name == "truncated" {
				t.Setenv("TV_TEST_SOURCE_PROBE", strings.Replace(validProbe, `"duration":"1.0"`, `"duration":"100.0"`, 1))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "cancel" {
				cancel()
			}
			if prepareErr := prepareFile(ctx, source, 0, tvLevel); prepareErr == nil {
				t.Fatal("expected error")
			}
			current, _ := os.ReadFile(source)
			if !bytes.Equal(current, original) {
				t.Fatal("original changed on failure")
			}
			assertNoTemps(t, source)
		})
	}
}

func TestConversionArguments(t *testing.T) {
	args := strings.Join(conversionArgs("input.mp4", "output.mp4", 720, tvLevel, preparationPlan{videoEncode: true, audioEncode: true}), " ")
	for _, part := range []string{"-map 0:v:0 -map 0:a:0 -sn -dn", "-c:v libx264 -preset veryfast -crf 22 -threads 2", "-profile:v main -level:v 4.1 -pix_fmt yuv420p -refs 3 -bf 2 -g 60", "open-gop=0 -maxrate 4M -bufsize 8M", "-r 30000/1001 -fps_mode cfr", "-c:a aac -profile:a aac_low -ac 2 -ar 48000 -b:a 160k", "-movflags +faststart -f mp4 output.mp4", "min(720,ih)"} {
		if !strings.Contains(args, part) {
			t.Errorf("missing %q in %s", part, args)
		}
	}
	for _, c := range []struct{ in, want int }{{0, 1080}, {2160, 1080}, {720, 720}, {481, 480}} {
		if got := heightLimit(c.in); got != c.want {
			t.Errorf("heightLimit(%d)=%d", c.in, got)
		}
	}
}

//nolint:gocyclo // Exercise all selective actions with independent preservation and no-op assertions.
func TestRealFFmpegPreparation(t *testing.T) {
	const videoOnly = "video only"
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	for _, kind := range []string{"remux", "audio only", videoOnly} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			source := filepath.Join(t.TempDir(), "movie.mp4")
			rate := "25"
			channels := "2"
			if kind == videoOnly {
				rate = "60"
			}
			if kind == "audio only" {
				channels = "1"
			}
			cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate="+rate,
				"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "1", "-c:v", "libx264", "-threads", "1",
				"-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", channels, source)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v: %s", err, out)
			}
			original, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			videoHash := streamHash(t, ctx, source, "0:v:0")
			audioHash := streamHash(t, ctx, source, "0:a:0")
			if prepareErr := prepareFile(ctx, source, 720, tvLevel); prepareErr != nil {
				t.Fatal(prepareErr)
			}
			info, err := probeMedia(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			if profileErr := validateProfile(info, 720, tvLevel); profileErr != nil {
				t.Fatal(profileErr)
			}
			video, _ := info.videoAudio()
			if video.Width > 320 || video.Height > 180 {
				t.Fatal("source was upscaled")
			}
			if kind != videoOnly && streamHash(t, ctx, source, "0:v:0") != videoHash {
				t.Fatal("supported video was reencoded")
			}
			if kind != "audio only" && streamHash(t, ctx, source, "0:a:0") != audioHash {
				t.Fatal("supported audio was reencoded")
			}
			if kind == videoOnly && (video.Profile != "Main" || math.Abs(frameRate(video.AvgFrameRate)-30000.0/1001) > 0.001) {
				t.Fatal("actual60fps source did not receive video fallback")
			}
			backups, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*", "movie.mp4"))
			if len(backups) != 1 {
				t.Fatalf("expected one backup, got %v", backups)
			}
			saved, err := os.ReadFile(backups[0])
			if err != nil || !bytes.Equal(saved, original) {
				t.Fatal("original backup mismatch")
			}
			prepared, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if prepareErr := prepareFile(ctx, source, 720, tvLevel); prepareErr != nil {
				t.Fatal(prepareErr)
			}
			current, err := os.ReadFile(source)
			if err != nil || !bytes.Equal(current, prepared) {
				t.Fatal("second preparation changed compatible output")
			}
			backups, _ = filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*", "movie.mp4"))
			if len(backups) != 1 {
				t.Fatal("compatible second pass created another backup")
			}
		})
	}
}

func streamHash(t *testing.T, ctx context.Context, path, stream string) string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-i", path, "-map", stream, "-c", "copy", "-f", "hash", "-hash", "sha256", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("stream hash: %v: %s", err, output)
	}
	return string(output)
}

func TestCancellationDuringFFmpegPreservesSource(t *testing.T) {
	source := fakeTools(t, "exec sleep 30\n")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if prepareErr := prepareFile(ctx, source, 0, tvLevel); prepareErr == nil {
		t.Fatal("canceled encode succeeded")
	}
	current, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("canceled encode changed original")
	}
	assertNoTemps(t, source)
}

const fakeProbeScript = `#!/bin/sh
case "$*" in *-show_packets*) printf '%s' "$TV_TEST_PACKETS"; exit;; esac
for input do :; done
case "$input" in */.tvcompat-*) printf '%s' "$TV_TEST_PROBE";; *) printf '%s' "${TV_TEST_SOURCE_PROBE:-$TV_TEST_PROBE}";; esac
`

func forceSourcePreparation(t *testing.T) {
	t.Helper()
	info := validMedia(t)
	info.Streams = append(info.Streams, mediaStream{CodecType: "subtitle", CodecName: "subrip"})
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TV_TEST_SOURCE_PROBE", string(data))
}

func TestSelectivePlanner(t *testing.T) {
	cases := []struct {
		name   string
		change func(*mediaInfo)
		fast   bool
		want   preparationPlan
	}{
		{"compatible", func(_ *mediaInfo) {}, true, preparationPlan{}},
		{"faststart only", func(_ *mediaInfo) {}, false, preparationPlan{remux: true}},
		{"supported High 44k", func(m *mediaInfo) { m.Streams[0].Profile = "High"; m.Streams[1].SampleRate = "44100" }, true, preparationPlan{}},
		{"nominal 90000", func(m *mediaInfo) { m.Streams[0].RFrameRate = "90000/1" }, true, preparationPlan{}},
		{"audio only", func(m *mediaInfo) { m.Streams[1].CodecName = "opus" }, true, preparationPlan{audioEncode: true}},
		{"video only", func(m *mediaInfo) { m.Streams[0].Level = 51 }, true, preparationPlan{videoEncode: true}},
		{"actual high fps", func(m *mediaInfo) { m.Streams[0].AvgFrameRate = "60/1" }, true, preparationPlan{videoEncode: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := validMedia(t)
			c.change(info)
			plan, err := planPreparation(info, c.fast, defaultTVHeight, tvLevel)
			if err != nil || plan != c.want {
				t.Fatalf("plan=%+v err=%v want=%+v", plan, err, c.want)
			}
		})
	}
	lowerLevelMedia := validMedia(t)
	lowerPlan, lowerErr := planPreparation(lowerLevelMedia, true, defaultTVHeight, 40)
	if lowerErr != nil || !lowerPlan.videoEncode {
		t.Fatal("explicit lower H264 level did not require true video encoding")
	}
	for _, plan := range []preparationPlan{{remux: true}, {audioEncode: true}, {videoEncode: true}} {
		args := strings.Join(conversionArgs("input", "output", 1080, 40, plan), " ")
		if strings.Contains(args, "-c:v libx264") != plan.videoEncode {
			t.Fatalf("unexpected video encode: %s", args)
		}
		if strings.Contains(args, "-c:a aac") != plan.audioEncode {
			t.Fatalf("unexpected audio encode: %s", args)
		}
		if !plan.videoEncode && !strings.Contains(args, "-c:v copy") {
			t.Fatal("supported video not copied")
		}
		if !plan.audioEncode && !strings.Contains(args, "-c:a copy") {
			t.Fatal("supported audio not copied")
		}
		if plan.videoEncode && !strings.Contains(args, "-level:v 4.0") {
			t.Fatal("explicit lower H264 level ignored")
		}
	}
}

func TestCompatibleFileIsNotChanged(t *testing.T) {
	source := fakeTools(t, "exit 99\n")
	t.Setenv("TV_TEST_PROBE", strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(validProbe, `"profile":"Main"`, `"profile":"High"`), `"r_frame_rate":"30000/1001"`, `"r_frame_rate":"90000/1"`), `"sample_rate":"48000"`, `"sample_rate":"44100"`))
	writeTestFile(t, source, []byte("\000\000\000\010ftyp\000\000\000\010moov\000\000\000\010mdat"), 0o600)
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if prepareErr := prepareFile(context.Background(), source, 0, tvLevel); prepareErr != nil {
		t.Fatal(prepareErr)
	}
	current, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("compatible file changed")
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*"))
	if len(backups) != 0 {
		t.Fatal("no-op created backup")
	}
	assertNoTemps(t, source)
}

func TestTimingSamples(t *testing.T) {
	cases := []struct {
		name      string
		times     []float64
		want      timingAssessment
		uncertain bool
	}{
		{"30fps", []float64{0, 0.033367, 0.066733, 0.100100}, timingSupported, false},
		{"60fps", []float64{0, 0.016667, 0.033333}, timingAboveLimit, false},
		{"B frame packet order", []float64{0.100100, 0, 0.066733, 0.033367}, timingSupported, false},
		{"empty", nil, timingSupported, true},
		{"one timestamp", []float64{1}, timingSupported, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := assessPacketTimes(c.times)
			if result != c.want || (err != nil) != c.uncertain {
				t.Fatalf("assessment=%v err=%v", result, err)
			}
		})
	}
	source := fakeTools(t, "exit 99\n")
	t.Setenv("TV_TEST_PACKETS", `{"packets":[]}`)
	if err := prepareFile(context.Background(), source, 0, tvLevel); err == nil || !strings.Contains(err.Error(), "verification inconclusive") {
		t.Fatalf("uncertain timing: %v", err)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "the original" {
		t.Fatal("uncertainty changed original")
	}
}

func TestUnsupportedH264LevelPreservesSource(t *testing.T) {
	source := fakeTools(t, "exit 99\n")
	for _, setting := range []string{"3.1", "3.2", "5.1", "4.x"} {
		if _, err := configuredTVLevel(setting); err == nil {
			t.Fatalf("unsafe setting %q accepted", setting)
		}
	}
	if err := prepareFile(context.Background(), source, 0, 31); err == nil {
		t.Fatal("1080p falsely labeled level3.1")
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "the original" {
		t.Fatal("unsupported level changed source")
	}
	for setting, want := range map[string]int{"": 41, "4.0": 40, "4.1": 41} {
		got, err := configuredTVLevel(setting)
		if err != nil || got != want {
			t.Fatalf("level %q=%d err=%v", setting, got, err)
		}
	}
	assertNoTemps(t, source)
}
