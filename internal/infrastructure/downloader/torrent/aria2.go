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

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

const (
	sigintTimeout       = 3 * time.Second
	sigkillTimeout      = 5 * time.Second
	aria2ExitUnfinished = 7  // aria2c exit code for unfinished downloads
	signalExitTerm      = 15 // SIGTERM exit code
	signalExitKill      = 9  // SIGKILL exit code
)

type Aria2Downloader struct {
	bot             domain.BotInterface
	torrentFileName string
	downloadDir     string
	cmd             *exec.Cmd
	stoppedManually bool
	config          *domain.Config
}

func NewAria2Downloader(botInstance domain.BotInterface, torrentFileName, moviePath string, cfg *domain.Config) domain.Downloader {
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

	logger.Log.WithFields(map[string]any{
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
			logger.Log.Errorf("aria2c stderr: %s", scanner.Text())
		}
	}()

	progressChan = make(chan float64)
	errChan = make(chan error, 1)

	go func() {
		defer close(progressChan)
		defer close(errChan)

		// Запускаем парсинг прогресса в отдельной горутине
		progressDone := make(chan struct{})
		go func() {
			defer close(progressDone)
			d.parseProgress(stdout, progressChan)
		}()

		// Ждем завершения процесса
		err := d.cmd.Wait()

		// Ждем завершения парсинга прогресса
		<-progressDone

		// Отправляем результат в канал ошибок
		if err != nil && !d.stoppedManually {
			logger.Log.WithError(err).Warn("aria2c exited with error")
			errChan <- err
		} else {
			logger.Log.Info("aria2c completed successfully")
			errChan <- nil
		}
	}()

	return progressChan, errChan, nil
}

func (*Aria2Downloader) parseProgress(r io.Reader, progressChan chan float64) {
	reProgress := regexp.MustCompile(`\(\s*(\d+\.?\d*)%\s*\)`)
	scanner := bufio.NewScanner(r)

	logger.Log.Info("Starting aria2 output parsing")

	for scanner.Scan() {
		line := scanner.Text()
		logger.Log.Debugf("aria2c output: %s", line)

		matches := reProgress.FindStringSubmatch(line)
		if len(matches) > 1 {
			if prog, err := strconv.ParseFloat(matches[1], 64); err == nil {
				logger.Log.WithField("progress", prog).Debug("Sending progress update")
				progressChan <- prog
				logger.Log.WithField("progress", prog).Debug("Progress update sent")
			} else {
				logger.Log.WithError(err).Error("failed to parse progress value")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Log.WithError(err).Error("error reading aria2c output")
	}

	logger.Log.Info("Finished aria2 output parsing")
}

func (d *Aria2Downloader) StopDownload() error {
	d.stoppedManually = true
	if d.cmd != nil && d.cmd.Process != nil {
		if err := d.cmd.Process.Signal(os.Interrupt); err != nil {
			return fmt.Errorf("failed to send SIGINT to aria2c process: %w", err)
		}
		logger.Log.Info("Sent SIGINT to aria2c process")

		done := make(chan error, 1)
		go func() {
			done <- d.cmd.Wait()
		}()
		select {
		case err := <-done:
			return d.handleProcessExit(err)
		case <-time.After(sigintTimeout):
			return d.handleSigkill(done)
		}
	}
	return nil
}

func (d *Aria2Downloader) StoppedManually() bool {
	return d.stoppedManually
}

// handleProcessExit handles the exit of aria2c process after SIGINT
func (d *Aria2Downloader) handleProcessExit(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, os.ErrProcessDone) || strings.Contains(err.Error(), "no child process") {
		return nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			exitCode := status.ExitStatus()
			if d.isExpectedExitCode(exitCode) {
				return nil
			}
		}
	}

	return fmt.Errorf("failed to wait for aria2c process to exit: %w", err)
}

// handleSigkill handles the SIGKILL scenario when SIGINT didn't work
func (d *Aria2Downloader) handleSigkill(done chan error) error {
	logger.Log.Warn("aria2c did not exit after SIGINT, sending SIGKILL...")
	if err := d.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to send SIGKILL to aria2c process: %w", err)
	}
	logger.Log.Info("Sent SIGKILL to aria2c process")

	// Wait for process with timeout to avoid infinite blocking
	select {
	case err := <-done:
		// After SIGKILL, any exit error is expected and not a real error
		if err != nil {
			logger.Log.WithError(err).Debug("aria2c process exited with error after SIGKILL (expected)")
		} else {
			logger.Log.Debug("aria2c process stopped gracefully after SIGKILL")
		}
		return nil
	case <-time.After(sigkillTimeout):
		logger.Log.Info("aria2c process did not exit within timeout, considering it stopped")
		return nil
	}
}

// isExpectedExitCode checks if the exit code is expected for a manually stopped process
func (*Aria2Downloader) isExpectedExitCode(exitCode int) bool {
	switch exitCode {
	case aria2ExitUnfinished:
		logger.Log.Debug("aria2c exited with code for unfinished downloads after manual stop")
		return true
	case signalExitTerm, signalExitKill:
		logger.Log.Debug("aria2c exited due to signal after manual stop")
		return true
	default:
		return false
	}
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
		logger.Log.WithError(err).Warn("Failed to parse torrent metadata for file size, returning 0")
		return 0, nil
	}

	if len(meta.Info.Files) > 0 {
		var totalSize int64
		for _, file := range meta.Info.Files {
			totalSize += file.Length
		}
		if logger.Log != nil {
			logger.Log.Debugf("Calculated total size for multi-file torrent: %d bytes", totalSize)
		}
		return totalSize, nil
	}

	if meta.Info.Length == 0 {
		if logger.Log != nil {
			logger.Log.Warn("Torrent metadata does not indicate file size, returning 0")
		}
		return 0, nil
	}
	if logger.Log != nil {
		logger.Log.Debugf("Single file torrent size: %d bytes", meta.Info.Length)
	}
	return meta.Info.Length, nil
}

func (d *Aria2Downloader) parseTorrentMeta() (*Meta, error) {
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)

	if err := d.validateTorrentFile(torrentPath); err != nil {
		return nil, fmt.Errorf("torrent file validation failed: %w", err)
	}

	return ParseMeta(torrentPath)
}

func (*Aria2Downloader) validateTorrentFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot get file stats: %w", err)
	}

	if stat.Size() < MinTorrentSize {
		return fmt.Errorf("file too small to be a valid torrent (%d bytes)", stat.Size())
	}

	if stat.Size() > MaxTorrentSize {
		return fmt.Errorf("file too large to be a torrent (%d bytes)", stat.Size())
	}

	headerSize := HeaderSize
	if stat.Size() < int64(headerSize) {
		headerSize = int(stat.Size())
	}

	header := make([]byte, headerSize)
	n, err := file.Read(header)
	if err != nil {
		return fmt.Errorf("cannot read file header: %w", err)
	}

	if n < 1 {
		return fmt.Errorf("file is empty")
	}

	return ValidateContent(header, n)
}

func (d *Aria2Downloader) buildAria2Args(torrentPath string, cfg *domain.Aria2Config) []string {
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

	logger.Log.WithField("aria2_args", args).Debug("Built aria2c command arguments")
	return args
}
