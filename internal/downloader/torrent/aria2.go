package aria2

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/jackpal/bencode-go"
)

type TorrentMeta struct {
	Info struct {
		Name   string `bencode:"name"`
		Length int64  `bencode:"length"`
		Files  []struct {
			Length int64    `bencode:"length"`
			Path   []string `bencode:"path"`
		} `bencode:"files"`
	} `bencode:"info"`
}

type Aria2Downloader struct {
	bot             *bot.Bot
	torrentFileName string
	downloadDir     string
	cmd             *exec.Cmd
	stoppedManually bool
	config          *config.Config
}

func NewAria2Downloader(botInstance *bot.Bot, torrentFileName, moviePath string, cfg *config.Config) downloader.Downloader {
	return &Aria2Downloader{
		bot:             botInstance,
		torrentFileName: torrentFileName,
		downloadDir:     moviePath,
		config:          cfg,
	}
}

func (d *Aria2Downloader) StartDownload(ctx context.Context) (progressChan chan float64, errChan chan error, err error) {
	aria2Cfg := d.config.GetAria2Settings()
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)

	logutils.Log.WithFields(map[string]any{
		"torrent_file": d.torrentFileName,
		"download_dir": d.downloadDir,
		"aria2_config": aria2Cfg,
	}).Info("Starting aria2c download with optimized configuration")

	cmdArgs := d.buildAria2Args(torrentPath, &aria2Cfg)
	d.cmd = exec.CommandContext(ctx, "aria2c", cmdArgs...)

	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture stdout: %w", err)
	}

	stderr, err := d.cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture stderr: %w", err)
	}

	if err := d.cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start aria2c: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logutils.Log.Errorf("aria2c stderr: %s", scanner.Text())
		}
	}()

	progressChan = make(chan float64)
	errChan = make(chan error, 1)

	go func() {
		defer close(progressChan)
		defer close(errChan)

		go d.parseProgress(stdout, progressChan)

		if err := d.cmd.Wait(); err != nil && !d.stoppedManually {
			logutils.Log.WithError(err).Warn("aria2c exited with error")
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	return progressChan, errChan, nil
}

func (*Aria2Downloader) parseProgress(r io.Reader, progressChan chan float64) {
	reProgress := regexp.MustCompile(`\(\s*(\d+\.?\d*)%\s*\)`)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		logutils.Log.Debugf("aria2c output: %s", line)

		matches := reProgress.FindStringSubmatch(line)
		if len(matches) > 1 {
			if prog, err := strconv.ParseFloat(matches[1], 64); err == nil {
				progressChan <- prog
			} else {
				logutils.Log.WithError(err).Error("failed to parse progress value")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logutils.Log.WithError(err).Error("error reading aria2c output")
	}
}

func (d *Aria2Downloader) StopDownload() error {
	d.stoppedManually = true
	if d.cmd != nil && d.cmd.Process != nil {
		if err := d.cmd.Process.Signal(os.Interrupt); err != nil {
			return fmt.Errorf("failed to send SIGINT to aria2c process: %w", err)
		}
		logutils.Log.Info("Sent SIGINT to aria2c process")

		done := make(chan error, 1)
		go func() {
			done <- d.cmd.Wait()
		}()
		select {
		case err := <-done:
			if err != nil {
				if errors.Is(err, os.ErrProcessDone) || strings.Contains(err.Error(), "no child process") {
					return nil
				}
				if exitErr, ok := err.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 7 {
						logutils.Log.Infof("aria2c exited with code 7 (unfinished downloads) after manual stop — not an error")
						return nil
					}
				}
				return fmt.Errorf("failed to wait for aria2c process to exit: %w", err)
			}
			return nil
		case <-time.After(3 * time.Second):
			logutils.Log.Warn("aria2c did not exit after SIGINT, sending SIGKILL...")
			if err := d.cmd.Process.Kill(); err != nil {
				return fmt.Errorf("failed to send SIGKILL to aria2c process: %w", err)
			}
			logutils.Log.Info("Sent SIGKILL to aria2c process")
			return <-done
		}
	}
	return nil
}

func (d *Aria2Downloader) StoppedManually() bool {
	return d.stoppedManually
}

func (d *Aria2Downloader) GetTitle() (string, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		return "", err
	}
	return meta.Info.Name, nil
}

func (d *Aria2Downloader) GetFiles() (mainFiles, tempFiles []string, err error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		return nil, nil, err
	}

	if len(meta.Info.Files) > 0 {
		for _, file := range meta.Info.Files {
			if meta.Info.Name == "" {
				return nil, nil, fmt.Errorf("torrent meta does not contain a root directory name")
			}
			if len(file.Path) == 0 {
				return nil, nil, fmt.Errorf("file path is empty in torrent meta")
			}
			mainFiles = append(mainFiles, filepath.Join(meta.Info.Name, filepath.Join(file.Path...)))
		}
	} else {
		mainFiles = append(mainFiles, meta.Info.Name)
	}

	tempFiles = []string{
		meta.Info.Name + ".aria2",
		d.torrentFileName,
	}

	return mainFiles, tempFiles, nil
}

func (d *Aria2Downloader) GetFileSize() (int64, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to parse torrent metadata for file size, returning 0")
		return 0, nil
	}

	if len(meta.Info.Files) > 0 {
		var totalSize int64
		for _, file := range meta.Info.Files {
			totalSize += file.Length
		}
		if logutils.Log != nil {
			logutils.Log.Debugf("Calculated total size for multi-file torrent: %d bytes", totalSize)
		}
		return totalSize, nil
	}

	if meta.Info.Length == 0 {
		if logutils.Log != nil {
			logutils.Log.Warn("Torrent metadata does not indicate file size, returning 0")
		}
		return 0, nil
	}
	if logutils.Log != nil {
		logutils.Log.Debugf("Single file torrent size: %d bytes", meta.Info.Length)
	}
	return meta.Info.Length, nil
}

func (d *Aria2Downloader) parseTorrentMeta() (*TorrentMeta, error) {
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)

	if err := d.validateTorrentFile(torrentPath); err != nil {
		return nil, fmt.Errorf("torrent file validation failed: %w", err)
	}

	f, err := os.Open(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open torrent file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			logutils.Log.WithError(cerr).Error("Failed to close torrent file")
		}
	}()

	var meta TorrentMeta
	if err := bencode.Unmarshal(f, &meta); err != nil {
		return nil, fmt.Errorf("failed to decode torrent meta: %w", err)
	}
	if meta.Info.Name == "" {
		return nil, fmt.Errorf("torrent meta does not contain a file name")
	}
	return &meta, nil
}

const (
	torrentHeaderSize = 16
	minTorrentSize    = 20
	maxTorrentSize    = 10 * 1024 * 1024
)

func (*Aria2Downloader) validateTorrentFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	header := make([]byte, torrentHeaderSize)
	n, err := file.Read(header)
	if err != nil {
		return fmt.Errorf("cannot read file header: %w", err)
	}

	if n < 1 {
		return fmt.Errorf("file is empty")
	}

	if header[0] != 'd' {
		if header[0] == '<' {
			return fmt.Errorf("file appears to be HTML, not a torrent file")
		}
		return fmt.Errorf("invalid torrent file format (expected bencode dictionary)")
	}

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot get file stats: %w", err)
	}

	if stat.Size() < minTorrentSize {
		return fmt.Errorf("file too small to be a valid torrent (%d bytes)", stat.Size())
	}

	if stat.Size() > maxTorrentSize {
		return fmt.Errorf("file too large to be a torrent (%d bytes)", stat.Size())
	}

	return nil
}

func (d *Aria2Downloader) buildAria2Args(torrentPath string, cfg *config.Aria2Config) []string {
	args := []string{
		"--dir", d.downloadDir,
		"--summary-interval=3",
		"--console-log-level=info",
		fmt.Sprintf("--file-allocation=%s", cfg.FileAllocation),
		"--allow-overwrite=false",
		"--auto-file-renaming=true",
		"--parameterized-uri=true",
	}

	args = append(args,
		fmt.Sprintf("--max-connection-per-server=%d", cfg.MaxConnectionsPerServer),
		fmt.Sprintf("--split=%d", cfg.Split),
		fmt.Sprintf("--min-split-size=%s", cfg.MinSplitSize),
		fmt.Sprintf("--max-concurrent-downloads=%d", 1), // Per torrent
		"--max-overall-download-limit=0",                // No global limit
		fmt.Sprintf("--bt-max-peers=%d", cfg.BTMaxPeers),
		fmt.Sprintf("--bt-request-peer-speed-limit=%s", cfg.BTRequestPeerSpeedLimit),
		fmt.Sprintf("--bt-max-open-files=%d", cfg.BTMaxOpenFiles),
		fmt.Sprintf("--max-overall-upload-limit=%s", cfg.MaxOverallUploadLimit),
		fmt.Sprintf("--max-upload-limit=%s", cfg.MaxUploadLimit),
		fmt.Sprintf("--seed-ratio=%.1f", cfg.SeedRatio),
		fmt.Sprintf("--seed-time=%d", cfg.SeedTime),
		fmt.Sprintf("--bt-tracker-timeout=%d", cfg.BTTrackerTimeout),
		fmt.Sprintf("--bt-tracker-interval=%d", cfg.BTTrackerInterval),
	)

	if cfg.EnableDHT {
		args = append(args,
			"--enable-dht=true",
			fmt.Sprintf("--dht-listen-port=%s", cfg.DHTPorts),
			"--dht-entry-point=dht.transmissionbt.com:6881",
			"--dht-entry-point6=dht.transmissionbt.com:6881",
		)
	} else {
		args = append(args, "--enable-dht=false")
	}

	if cfg.EnablePeerExchange {
		args = append(args, "--enable-peer-exchange=true")
	} else {
		args = append(args, "--enable-peer-exchange=false")
	}

	if cfg.EnableLocalPeerDiscovery {
		args = append(args, "--bt-enable-lpd=true")
	} else {
		args = append(args, "--bt-enable-lpd=false")
	}

	// Listen port settings
	args = append(args, fmt.Sprintf("--listen-port=%s", cfg.ListenPort))

	// Additional torrent settings
	if cfg.BTSaveMetadata {
		args = append(args, "--bt-save-metadata=true")
	}

	if cfg.BTHashCheckSeed {
		args = append(args, "--bt-hash-check-seed=true")
	}

	if cfg.BTRequireCrypto {
		args = append(args,
			"--bt-require-crypto=true",
			fmt.Sprintf("--bt-min-crypto-level=%s", cfg.BTMinCryptoLevel),
		)
	} else {
		args = append(args, "--bt-require-crypto=false")
	}

	if cfg.CheckIntegrity {
		args = append(args, "--check-integrity=true")
	}

	if cfg.ContinueDownload {
		args = append(args, "--continue=true")
	}

	if cfg.RemoteTime {
		args = append(args, "--remote-time=true")
	}

	// Network and proxy settings
	if cfg.HTTPProxy != "" {
		args = append(args, fmt.Sprintf("--http-proxy=%s", cfg.HTTPProxy))
	}

	if cfg.AllProxy != "" {
		args = append(args, fmt.Sprintf("--all-proxy=%s", cfg.AllProxy))
	}

	if cfg.UserAgent != "" {
		args = append(args, fmt.Sprintf("--user-agent=%s", cfg.UserAgent))
	}

	// Timeout and retry settings, plus fallback trackers for better connectivity
	args = append(args,
		fmt.Sprintf("--timeout=%d", cfg.Timeout),
		fmt.Sprintf("--max-tries=%d", cfg.MaxTries),
		fmt.Sprintf("--retry-wait=%d", cfg.RetryWait),
		"--bt-tracker=udp://tracker.opentrackr.org:1337/announce",
		"--bt-tracker=udp://9.rarbg.to:2920/announce",
		"--bt-tracker=udp://tracker.openbittorrent.com:80/announce",
		"--bt-tracker=udp://exodus.desync.com:6969/announce",
		"--bt-tracker=udp://tracker.torrent.eu.org:451/announce",
		"--bt-tracker=udp://tracker.coppersurfer.tk:6969/announce",
		"--bt-tracker=udp://tracker.leechers-paradise.org:6969/announce",
		"--bt-tracker=udp://zer0day.ch:1337/announce",
		"--bt-tracker=udp://open.demonii.si:1337/announce",
	)

	// Follow torrent setting
	if cfg.FollowTorrent {
		args = append(args, "--follow-torrent=true")
	}

	// Add the torrent file path at the end
	args = append(args, torrentPath)

	logutils.Log.WithField("aria2_args", args).Debug("Built aria2c command arguments")
	return args
}
