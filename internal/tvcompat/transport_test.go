package tvcompat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/database"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/testutils"
)

const unrelatedSubtitle = "unrelated.srt"

func mp4TestAtom(kind string, payload []byte) []byte {
	atom := make([]byte, mp4AtomHeader+len(payload))
	// #nosec G115 -- these fixture atoms contain only small fixed-size metadata.
	binary.BigEndian.PutUint32(atom[:4], uint32(len(atom)))
	copy(atom[4:8], kind)
	copy(atom[8:], payload)
	return atom
}

func sparseMP4(t *testing.T, path, offsetKind string, large bool) {
	t.Helper()
	payload := mp4TestAtom(offsetKind, make([]byte, 16))
	for _, kind := range []string{"stbl", "minf", "mdia", "trak", "moov"} {
		payload = mp4TestAtom(kind, payload)
	}
	data := append(mp4TestAtom("ftyp", nil), payload...)
	data = append(data, []byte{0, 0, 0, 0, 'm', 'd', 'a', 't'}...)
	writeTestFile(t, path, data, 0o600)
	if large {
		if err := os.Truncate(path, legacyMP4SizeLimit+1024); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLargeMP4Co64Routing(t *testing.T) {
	for _, c := range []struct {
		name, offset string
		large, want  bool
	}{
		{"large co64", "co64", true, true}, {"large stco", "stco", true, false}, {"small co64", "co64", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "movie.mp4")
			sparseMP4(t, path, c.offset, c.large)
			got, err := needsTransportStream(path)
			if err != nil || got != c.want {
				t.Fatalf("routing=%v err=%v", got, err)
			}
		})
	}
	if _, err := containsCo64(bytes.NewReader([]byte{0, 0, 0, 2, 'm', 'o', 'o', 'v'}), 0, 8, 0); err == nil {
		t.Fatal("malformed atom accepted")
	}
}

func fakeTransportTools(t *testing.T) (source, arguments string) {
	t.Helper()
	source = fakeTools(t, `printf '%s\n' "$@" >> "$TV_TRANSPORT_ARGS"
for output do :; done
printf 'transport payload' > "$output"
`)
	arguments = filepath.Join(filepath.Dir(source), "arguments")
	t.Setenv("TV_TRANSPORT_ARGS", arguments)
	sourceProbe := strings.ReplaceAll(strings.ReplaceAll(validProbe, `"profile":"Main"`, `"profile":"High"`), `"sample_rate":"48000"`, `"sample_rate":"44100"`)
	t.Setenv("TV_MP4_PROBE", sourceProbe)
	t.Setenv("TV_TS_PROBE", strings.Replace(sourceProbe, `"format_name":"mov,mp4,m4a,3gp,3g2,mj2"`, `"format_name":"mpegts"`, 1))
	toolDir := strings.Split(os.Getenv("PATH"), ":")[0]
	script := `#!/bin/sh
case "$*" in *-show_packets*) printf '%s' "$TV_TEST_PACKETS"; exit;; esac
for input do :; done
case "$input" in *.ts) printf '%s' "$TV_TS_PROBE";; *) printf '%s' "$TV_MP4_PROBE";; esac
`
	writeTestFile(t, filepath.Join(toolDir, "ffprobe"), []byte(script), 0o700)
	return source, arguments
}

func transportMovie(t *testing.T, source string) (store database.Database, movieID uint, mainFiles []database.MovieFile) {
	t.Helper()
	logutils.InitLogger("error")
	db := database.NewSQLiteDatabase()
	if err := db.Init(testutils.TestConfig(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	id, err := db.AddMovie(context.Background(), "video", 0, []string{filepath.Base(source), unrelatedSubtitle}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	files, err := db.GetFilesByMovieID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return db, id, files
}

//nolint:gocyclo // Check independent publication, preservation, DB, collision and second-pass guarantees.
func TestTransportPublicationPreservesSourceAndExistingMaterial(t *testing.T) {
	source, arguments := fakeTransportTools(t)
	sparseMP4(t, source, "co64", true)
	original, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	existing := strings.TrimSuffix(source, ".mp4") + ".ts"
	writeTestFile(t, existing, []byte("unrelated existing TS"), 0o600)
	db, id, _ := transportMovie(t, source)
	cfg := &config.VideoConfig{CompatibilityMode: true}
	if runErr := RunTvCompatibility(context.Background(), id, filepath.Dir(source), db, cfg); runErr != nil {
		t.Fatal(runErr)
	}
	files, err := db.GetFilesByMovieID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	prepared := ""
	for _, file := range files {
		if file.FilePath != unrelatedSubtitle {
			prepared = filepath.Join(filepath.Dir(source), file.FilePath)
		}
	}
	if filepath.Ext(prepared) != ".ts" || prepared == existing {
		t.Fatalf("new TS path not registered exclusively: %s", prepared)
	}
	current, err := os.ReadFile(existing)
	if err != nil || string(current) != "unrelated existing TS" {
		t.Fatal("existing material overwritten")
	}
	if _, statErr := os.Stat(source); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("old visible source remains after successful path update")
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*", "movie.mp4"))
	if len(backups) != 1 {
		t.Fatalf("original backup missing: %v", backups)
	}
	backup, err := os.Stat(backups[0])
	if err != nil || !os.SameFile(original, backup) {
		t.Fatal("original inode not retained")
	}
	args, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c:v\ncopy\n-c:a\ncopy\n") || strings.Contains(string(args), "libx264") {
		t.Fatalf("source was encoded: %s", args)
	}
	if runErr := RunTvCompatibility(context.Background(), id, filepath.Dir(source), db, cfg); runErr != nil {
		t.Fatal(runErr)
	}
	after, err := os.ReadFile(arguments)
	if err != nil || !bytes.Equal(args, after) {
		t.Fatal("compatible TS second pass invoked ffmpeg")
	}
}

type uncertainPreparedStore struct {
	database.Database
	commit bool
}

func (s uncertainPreparedStore) ReplaceMainMovieFilePath(ctx context.Context, movieID, fileID uint, oldPath, newPath string) error {
	if s.commit {
		if err := s.Database.(preparedPathStore).ReplaceMainMovieFilePath(ctx, movieID, fileID, oldPath, newPath); err != nil {
			return err
		}
	}
	return fmt.Errorf("simulated ambiguous DB result")
}

func TestTransportDBFailureNeverLosesReferencedMedia(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			source, _ := fakeTransportTools(t)
			sparseMP4(t, source, "co64", true)
			db, id, files := transportMovie(t, source)
			store := uncertainPreparedStore{Database: db, commit: committed}
			err := prepareMP4AsTransport(context.Background(), id, &files[0], filepath.Dir(source), store, 0, tvLevel)
			if err == nil {
				t.Fatal("DB failure hidden")
			}
			if _, statErr := os.Stat(source); statErr != nil {
				t.Fatal("original lost after DB failure")
			}
			rows, err := db.GetFilesByMovieID(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.FilePath == unrelatedSubtitle {
					continue
				}
				if _, err := os.Stat(filepath.Join(filepath.Dir(source), row.FilePath)); err != nil {
					t.Fatal("DB references a missing file")
				}
			}
			outputs, _ := filepath.Glob(filepath.Join(filepath.Dir(source), "*.ts"))
			if len(outputs) != 1 {
				t.Fatal("prepared material lost after ambiguous result")
			}
		})
	}
}

func TestTransportSelectiveStreamArguments(t *testing.T) {
	for _, plan := range []preparationPlan{{remux: true}, {audioEncode: true}, {videoEncode: true}} {
		args := strings.Join(transportArgs("input.mp4", "output.ts", 1080, tvLevel, plan), " ")
		if strings.Contains(args, "-c:v libx264") != plan.videoEncode || strings.Contains(args, "-c:a aac") != plan.audioEncode {
			t.Fatal(args)
		}
		if !strings.Contains(args, "-f mpegts output.ts") || strings.Contains(args, "faststart") {
			t.Fatal(args)
		}
	}
}

//nolint:gocyclo // Real media validation, original preservation and idempotence are independent checks.
func TestRealTransportStreamCopyAndSecondPass(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	source := filepath.Join(t.TempDir(), "movie.mp4")
	command := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "1", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	db, id, files := transportMovie(t, source)
	if prepareErr := prepareMP4AsTransport(ctx, id, &files[0], filepath.Dir(source), db, 0, tvLevel); prepareErr != nil {
		t.Fatal(prepareErr)
	}
	rows, err := db.GetFilesByMovieID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	prepared := ""
	for _, row := range rows {
		if row.FilePath != unrelatedSubtitle {
			prepared = filepath.Join(filepath.Dir(source), row.FilePath)
		}
	}
	info, err := probeMedia(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	video, audio := info.videoAudio()
	if info.Format.FormatName != "mpegts" || video.Profile != h264ProfileHigh || audio.SampleRate != "44100" {
		t.Fatalf("streams changed: video=%+v audio=%+v", video, audio)
	}
	if runErr := RunTvCompatibility(ctx, id, filepath.Dir(source), db, &config.VideoConfig{CompatibilityMode: true}); runErr != nil {
		t.Fatal(runErr)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-originals", "*", "movie.mp4"))
	if len(backups) != 1 {
		t.Fatal("missing original")
	}
	saved, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatal("original not retained")
	}
}

func TestLargeTransportAudioRepairCopiesSupportedVideo(t *testing.T) {
	source, arguments := fakeTransportTools(t)
	sparseMP4(t, source, "co64", true)
	t.Setenv("TV_MP4_PROBE", strings.Replace(os.Getenv("TV_MP4_PROBE"), `"channels":2`, `"channels":6`, 1))
	db, id, _ := transportMovie(t, source)
	if err := RunTvCompatibility(context.Background(), id, filepath.Dir(source), db, &config.VideoConfig{CompatibilityMode: true}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c:v\ncopy\n-c:a\naac\n") || strings.Contains(string(args), "libx264") {
		t.Fatalf("audio repair changed supported video: %s", args)
	}
}

func TestTransportCancellationRetainsSourceAndDBPath(t *testing.T) {
	source, _ := fakeTransportTools(t)
	sparseMP4(t, source, "co64", true)
	db, id, _ := transportMovie(t, source)
	started := filepath.Join(filepath.Dir(source), "started")
	t.Setenv("TV_TRANSPORT_STARTED", started)
	toolDir := strings.Split(os.Getenv("PATH"), ":")[0]
	writeTestFile(t, filepath.Join(toolDir, "ffmpeg"), []byte("#!/bin/sh\nprintf started > \"$TV_TRANSPORT_STARTED\"\nexec sleep 30\n"), 0o700)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunTvCompatibility(ctx, id, filepath.Dir(source), db, &config.VideoConfig{CompatibilityMode: true})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(started); err != nil {
		t.Fatal("TS remux did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled TS preparation succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("TS preparation ignored cancellation")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("canceled TS preparation removed source")
	}
	files, err := db.GetFilesByMovieID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.FilePath != unrelatedSubtitle && file.FilePath != "movie.mp4" {
			t.Fatal("cancellation changed DB path")
		}
	}
	outputs, _ := filepath.Glob(filepath.Join(filepath.Dir(source), "*.ts"))
	if len(outputs) != 0 {
		t.Fatal("canceled TS output published")
	}
	temps, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".tvcompat-*.ts"))
	if len(temps) != 0 {
		t.Fatal("canceled TS temporary file leaked")
	}
}

// Full-source probe shape observed during the real-device test: unspecified SAR,
// credible MP4 average FPS, and MPEG-TS reporting 0/0 despite valid packet PTS.
const fullSourceProbe = `{"streams":[{"codec_type":"video","codec_name":"h264","profile":"High","level":40,"width":1920,"height":1080,"pix_fmt":"yuv420p","field_order":"progressive","r_frame_rate":"90000/1","avg_frame_rate":"980450783/32714390"},{"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"sample_rate":"44100"}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"27506.6"}}`

func fullSourceTransportProbe() string {
	return strings.Replace(strings.Replace(fullSourceProbe, `"format_name":"mov,mp4,m4a,3gp,3g2,mj2"`, `"format_name":"mpegts"`, 1),
		`"avg_frame_rate":"980450783/32714390"`, `"avg_frame_rate":"0/0"`, 1)
}

func TestObservedFullSourceMetadataCopiesToTSWithoutEncoding(t *testing.T) {
	source, arguments := fakeTransportTools(t)
	sparseMP4(t, source, "co64", true)
	t.Setenv("TV_MP4_PROBE", fullSourceProbe)
	t.Setenv("TV_TS_PROBE", fullSourceTransportProbe())
	// Representative positive spacings within the observed 0.033356..0.083477 range.
	t.Setenv("TV_TEST_PACKETS", `{"packets":[{"pts_time":"0"},{"pts_time":"0.033356"},{"pts_time":"0.066712"},{"pts_time":"0.150189"}]}`)
	db, id, _ := transportMovie(t, source)
	cfg := &config.VideoConfig{CompatibilityMode: true}
	if err := RunTvCompatibility(context.Background(), id, filepath.Dir(source), db, cfg); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c:v\ncopy\n-c:a\ncopy\n") || strings.Contains(string(args), "libx264") {
		t.Fatalf("actual-shape input was encoded: %s", args)
	}
	if runErr := RunTvCompatibility(context.Background(), id, filepath.Dir(source), db, cfg); runErr != nil {
		t.Fatal(runErr)
	}
	after, err := os.ReadFile(arguments)
	if err != nil || !bytes.Equal(args, after) {
		t.Fatal("compatible TS with unknown average FPS was reprocessed")
	}
}

func TestUnknownTransportFPSStillRequiresValidPacketSamples(t *testing.T) {
	source, _ := fakeTransportTools(t)
	transport := strings.TrimSuffix(source, ".mp4") + ".ts"
	if err := os.Rename(source, transport); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TV_TS_PROBE", fullSourceTransportProbe())
	t.Setenv("TV_TEST_PACKETS", `{"packets":[]}`)
	if err := validateExistingTransport(context.Background(), transport, defaultTVHeight, tvLevel); err == nil || !strings.Contains(err.Error(), "verification inconclusive") {
		t.Fatalf("unverified unknown FPS accepted: %v", err)
	}
}
