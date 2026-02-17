package aria2

import (
	"os"
	"strings"
	"testing"
	"time"

	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
)

func TestMain(m *testing.M) {
	logutils.InitLogger("error")
	os.Exit(m.Run())
}

func TestParseProgress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []float64
	}{
		{
			name:     "Single progress line",
			input:    "[#abc123 1024B/2048B(50.0%)]",
			expected: []float64{50.0},
		},
		{
			name:     "Multiple progress lines",
			input:    "[#abc 100B/1000B(10%)]\n[#abc 500B/1000B(50%)]\n[#abc 1000B/1000B(100%)]\n",
			expected: []float64{10, 50, 100},
		},
		{
			name:     "Progress with decimal",
			input:    "[#abc 100B/300B(33.33%)]",
			expected: []float64{33.33},
		},
		{
			name:     "No progress in output",
			input:    "Downloading file...\nConnecting to tracker...\n",
			expected: nil,
		},
		{
			name:     "Mixed output with progress",
			input:    "Connecting...\n[#abc 256B/1024B(25%)]\nVerifying...\n[#abc 768B/1024B(75%)]\nDone.\n",
			expected: []float64{25, 75},
		},
		{
			name:     "Zero percent",
			input:    "[#abc 0B/1024B( 0%)]",
			expected: []float64{0},
		},
		{
			name:     "Progress with spaces around percent",
			input:    "[#abc 512B/1024B(  50.5% )]",
			expected: []float64{50.5},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Aria2Downloader{}
			reader := strings.NewReader(tt.input)
			progressChan := make(chan float64, 100)

			d.parseProgress(reader, progressChan)
			close(progressChan)

			var got []float64
			for p := range progressChan {
				got = append(got, p)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d progress values %v, want %d values %v", len(got), got, len(tt.expected), tt.expected)
			}

			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("progress[%d] = %f, want %f", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestParseProgress_ChannelReceivable(t *testing.T) {
	d := &Aria2Downloader{}
	input := "[#abc 512B/1024B(50%)]\n[#abc 1024B/1024B(100%)]\n"
	reader := strings.NewReader(input)
	progressChan := make(chan float64, 10)

	done := make(chan struct{})
	go func() {
		d.parseProgress(reader, progressChan)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("parseProgress did not finish within timeout")
	}

	close(progressChan)
	var values []float64
	for v := range progressChan {
		values = append(values, v)
	}
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d: %v", len(values), values)
	}
}

func TestIsExpectedExitCode(t *testing.T) {
	d := &Aria2Downloader{}

	tests := []struct {
		name     string
		exitCode int
		expected bool
	}{
		{"Aria2 unfinished exit code", aria2ExitUnfinished, true},
		{"SIGTERM exit code", signalExitTerm, true},
		{"SIGKILL exit code", signalExitKill, true},
		{"Normal exit code 0", 0, false},
		{"Unknown exit code 1", 1, false},
		{"Unknown exit code 2", 2, false},
		{"Unknown exit code 128", 128, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.isExpectedExitCode(tt.exitCode)
			if result != tt.expected {
				t.Errorf("isExpectedExitCode(%d) = %v, want %v", tt.exitCode, result, tt.expected)
			}
		})
	}
}

func TestBuildAria2Args(t *testing.T) {
	d := &Aria2Downloader{
		downloadDir: "/tmp/downloads",
	}

	baseCfg := &tmsconfig.Aria2Config{
		FileAllocation:          "none",
		MaxConnectionsPerServer: 4,
		Split:                   4,
		MinSplitSize:            "1M",
		BTMaxPeers:              100,
		BTRequestPeerSpeedLimit: "100K",
		BTMaxOpenFiles:          100,
		MaxOverallUploadLimit:   "10K",
		MaxUploadLimit:          "10K",
		SeedRatio:               0.0,
		SeedTime:                0,
		BTTrackerTimeout:        60,
		BTTrackerInterval:       60,
		ListenPort:              "6881",
		Timeout:                 60,
		MaxTries:                5,
		RetryWait:               0,
	}

	t.Run("Basic args", func(t *testing.T) {
		args := d.buildAria2Args("/tmp/test.torrent", baseCfg)

		if args[len(args)-1] != "/tmp/test.torrent" {
			t.Errorf("Last arg should be torrent path, got %s", args[len(args)-1])
		}

		assertContainsArg(t, args, "--dir")
		assertContainsArg(t, args, "--summary-interval=3")
	})

	t.Run("DHT enabled", func(t *testing.T) {
		cfgDHT := *baseCfg
		cfgDHT.EnableDHT = true
		cfgDHT.DHTPorts = "6881-6999"
		args := d.buildAria2Args("/tmp/test.torrent", &cfgDHT)
		assertContainsArg(t, args, "--enable-dht=true")
	})

	t.Run("DHT disabled", func(t *testing.T) {
		cfgNoDHT := *baseCfg
		cfgNoDHT.EnableDHT = false
		args := d.buildAria2Args("/tmp/test.torrent", &cfgNoDHT)
		assertContainsArg(t, args, "--enable-dht=false")
	})

	t.Run("Proxy settings", func(t *testing.T) {
		cfgProxy := *baseCfg
		cfgProxy.HTTPProxy = "http://proxy:8080"
		cfgProxy.AllProxy = "socks5://proxy:1080"
		args := d.buildAria2Args("/tmp/test.torrent", &cfgProxy)
		assertContainsArg(t, args, "--http-proxy=http://proxy:8080")
		assertContainsArg(t, args, "--all-proxy=socks5://proxy:1080")
	})

	t.Run("User agent", func(t *testing.T) {
		cfgUA := *baseCfg
		cfgUA.UserAgent = "TestAgent/1.0"
		args := d.buildAria2Args("/tmp/test.torrent", &cfgUA)
		assertContainsArg(t, args, "--user-agent=TestAgent/1.0")
	})

	t.Run("Peer exchange enabled", func(t *testing.T) {
		cfgPex := *baseCfg
		cfgPex.EnablePeerExchange = true
		args := d.buildAria2Args("/tmp/test.torrent", &cfgPex)
		assertContainsArg(t, args, "--enable-peer-exchange=true")
	})

	t.Run("Local peer discovery disabled", func(t *testing.T) {
		cfgLpd := *baseCfg
		cfgLpd.EnableLocalPeerDiscovery = false
		args := d.buildAria2Args("/tmp/test.torrent", &cfgLpd)
		assertContainsArg(t, args, "--bt-enable-lpd=false")
	})

	t.Run("Crypto required", func(t *testing.T) {
		cfgCrypto := *baseCfg
		cfgCrypto.BTRequireCrypto = true
		cfgCrypto.BTMinCryptoLevel = "arc4"
		args := d.buildAria2Args("/tmp/test.torrent", &cfgCrypto)
		assertContainsArg(t, args, "--bt-require-crypto=true")
		assertContainsArg(t, args, "--bt-min-crypto-level=arc4")
	})

	t.Run("Continue download", func(t *testing.T) {
		cfgCont := *baseCfg
		cfgCont.ContinueDownload = true
		args := d.buildAria2Args("/tmp/test.torrent", &cfgCont)
		assertContainsArg(t, args, "--continue=true")
	})
}

func assertContainsArg(t *testing.T, args []string, expected string) {
	t.Helper()
	for _, arg := range args {
		if arg == expected {
			return
		}
	}
	t.Errorf("Expected arg %q not found in args", expected)
}
