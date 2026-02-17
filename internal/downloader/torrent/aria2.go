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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/tvcompat"
)

const (
	sigintTimeout       = 3 * time.Second
	sigkillTimeout      = 5 * time.Second
	aria2ExitUnfinished = 7   // aria2c exit code for unfinished downloads
	signalExitTerm      = 15  // SIGTERM exit code
	signalExitKill      = 9   // SIGKILL exit code
	maxProgressPercent  = 100 // cap for progress percentage
)

type Aria2Downloader struct {
	torrentFileName string
	downloadDir     string
	cmd             *exec.Cmd
	stoppedManually bool
	config          *config.Config
	episodeAck      <-chan struct{} // injected by manager; blocks between sequential episodes
}

// SetEpisodeAck implements downloader.EpisodeAcker.
func (d *Aria2Downloader) SetEpisodeAck(ack <-chan struct{}) {
	d.episodeAck = ack
}

func NewAria2Downloader(torrentFileName, moviePath string, cfg *config.Config) downloader.Downloader {
	return &Aria2Downloader{
		torrentFileName: torrentFileName,
		downloadDir:     moviePath,
		config:          cfg,
	}
}

func (d *Aria2Downloader) StartDownload(
	ctx context.Context,
) (progressChan chan float64, errChan chan error, episodesChan <-chan int, err error) {
	aria2Cfg := d.config.GetAria2Settings()
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)

	meta, metaErr := d.parseTorrentMeta()
	if metaErr != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse torrent meta: %w", metaErr)
	}

	progressChan = make(chan float64)
	errChan = make(chan error, 1)

	if len(meta.Info.Files) > 0 {
		epCh := make(chan int, len(meta.Info.Files))
		episodesChan = epCh
		go d.runMultiFileDownload(ctx, torrentPath, meta, &aria2Cfg, progressChan, errChan, epCh)
	} else {
		go d.runSingleDownload(ctx, torrentPath, &aria2Cfg, progressChan, errChan)
		return progressChan, errChan, nil, nil
	}
	return progressChan, errChan, episodesChan, nil
}

// runSingleDownload runs one aria2 process for single-file torrent or multi-file without ordering.
func (d *Aria2Downloader) runSingleDownload(
	ctx context.Context,
	torrentPath string,
	aria2Cfg *config.Aria2Config,
	progressChan chan float64,
	errChan chan error,
) {
	defer close(errChan)

	cmdArgs := d.buildAria2Args(torrentPath, aria2Cfg, nil)
	d.cmd = exec.CommandContext(ctx, "aria2c", cmdArgs...)

	stdout, pipeErr := d.cmd.StdoutPipe()
	if pipeErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to capture stdout: %w", pipeErr)
		return
	}
	stderr, pipeErr := d.cmd.StderrPipe()
	if pipeErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to capture stderr: %w", pipeErr)
		return
	}
	if startErr := d.cmd.Start(); startErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to start aria2c: %w", startErr)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logutils.Log.Errorf("aria2c stderr: %s", scanner.Text())
		}
	}()

	// parseProgress will close progressChan when done
	go d.parseProgressAndClose(stdout, progressChan)

	waitErr := d.cmd.Wait()
	if waitErr != nil && !d.stoppedManually {
		logutils.Log.WithError(waitErr).Warn("aria2c exited with error")
		errChan <- waitErr
	} else {
		errChan <- nil
	}
}

// runMultiFileDownload runs aria2 for multi-file torrent.
// By default: one process for all files (best total throughput).
// If aria2Cfg.SequentialMultiFile: run file-by-file (first file ready sooner; may reduce total speed).
func (d *Aria2Downloader) runMultiFileDownload(
	ctx context.Context,
	torrentPath string,
	meta *Meta,
	aria2Cfg *config.Aria2Config,
	progressChan chan float64,
	errChan chan error,
	episodesChan chan int,
) {
	defer close(errChan)
	if episodesChan != nil {
		defer close(episodesChan)
	}

	indices, sizes, isVideo, totalSize := sortedFileIndicesByPath(meta)
	if totalSize == 0 {
		close(progressChan)
		errChan <- nil
		return
	}

	totalVideo := 0
	for _, v := range isVideo {
		if v {
			totalVideo++
		}
	}

	if aria2Cfg.SequentialMultiFile {
		d.runMultiFileDownloadSequential(
			ctx,
			torrentPath,
			aria2Cfg,
			indices,
			sizes,
			isVideo,
			totalSize,
			totalVideo,
			progressChan,
			errChan,
			episodesChan,
		)
		close(progressChan)
		return
	}

	// Single run: all files, no --select-file — one swarm connection, best throughput
	logutils.Log.WithFields(map[string]any{
		"torrent_file":   d.torrentFileName,
		"download_dir":   d.downloadDir,
		"num_files":      len(isVideo),
		"video_episodes": totalVideo,
	}).Info("Starting aria2c multi-file download (single instance, all files)")

	cmdArgs := d.buildAria2Args(torrentPath, aria2Cfg, nil)
	cmd := exec.CommandContext(ctx, "aria2c", cmdArgs...)
	d.cmd = cmd

	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to capture stdout: %w", pipeErr)
		return
	}
	stderr, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to capture stderr: %w", pipeErr)
		return
	}
	if startErr := cmd.Start(); startErr != nil {
		close(progressChan)
		errChan <- fmt.Errorf("failed to start aria2c: %w", startErr)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logutils.Log.Errorf("aria2c stderr: %s", scanner.Text())
		}
	}()

	// parseProgressAndClose will close progressChan when done
	go d.parseProgressAndClose(stdout, progressChan)

	waitErr := cmd.Wait()
	if waitErr != nil && !d.stoppedManually {
		logutils.Log.WithError(waitErr).Warn("aria2c exited with error")
		errChan <- waitErr
		return
	}
	if d.stoppedManually {
		errChan <- downloader.ErrStoppedByUser
		return
	}
	if episodesChan != nil && totalVideo > 0 {
		episodesChan <- totalVideo
	}
	errChan <- nil
}

// runOneSequentialBatch runs aria2 for one file set (selectIndices), forwards progress to progressChan,
// and returns the number of video files in this batch or an error.
//
// Progress calculation: aria2 is invoked with --select-file covering all files up to the current batch.
// Files from previous batches are already complete, so aria2 reports its own progress starting from a non-zero
// value (e.g. 66% for batch 3 of 3 equal files). Thus runTotalSize*p/100 already accounts for all completed
// bytes, and no separate "completedSize" offset is needed.
func (d *Aria2Downloader) runOneSequentialBatch(
	ctx context.Context,
	torrentPath string,
	aria2Cfg *config.Aria2Config,
	selectIndices []int,
	runTotalSize int64,
	totalSize int64,
	progressChan chan float64,
	isVideo []bool,
) (videoDone int, err error) {
	cmdArgs := d.buildAria2Args(torrentPath, aria2Cfg, selectIndices)
	cmd := exec.CommandContext(ctx, "aria2c", cmdArgs...)
	d.cmd = cmd

	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return 0, fmt.Errorf("failed to capture stdout: %w", pipeErr)
	}
	stderr, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		return 0, fmt.Errorf("failed to capture stderr: %w", pipeErr)
	}
	if startErr := cmd.Start(); startErr != nil {
		return 0, fmt.Errorf("failed to start aria2c: %w", startErr)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logutils.Log.Errorf("aria2c stderr: %s", scanner.Text())
		}
	}()

	runProgressChan := make(chan float64)
	go d.parseProgress(stdout, runProgressChan)
	go func() {
		for p := range runProgressChan {
			// aria2 progress (p) already includes previously completed files in this batch,
			// so overall = (bytes done in this run) / totalSize * 100.
			overall := float64(runTotalSize) * p / maxProgressPercent
			overall = overall / float64(totalSize) * maxProgressPercent
			if overall > maxProgressPercent {
				overall = maxProgressPercent
			}
			progressChan <- overall
		}
	}()

	waitErr := cmd.Wait()
	close(runProgressChan)
	if waitErr != nil && !d.stoppedManually {
		logutils.Log.WithError(waitErr).Warn("aria2c exited with error")
		return 0, waitErr
	}
	if d.stoppedManually {
		return 0, downloader.ErrStoppedByUser
	}

	for _, v := range isVideo {
		if v {
			videoDone++
		}
	}
	return videoDone, nil
}

// runMultiFileDownloadSequential runs aria2 once per file set: first --select-file=1, then 1,2, … so the first file completes sooner.
func (d *Aria2Downloader) runMultiFileDownloadSequential(
	ctx context.Context,
	torrentPath string,
	aria2Cfg *config.Aria2Config,
	indices []int,
	sizes []int64,
	isVideo []bool,
	totalSize int64,
	_ int, // totalVideo: reserved for future use
	progressChan chan float64,
	errChan chan error,
	episodesChan chan int,
) {
	logutils.Log.WithFields(map[string]any{
		"torrent_file": d.torrentFileName,
		"num_files":    len(indices),
	}).Info("Starting aria2c multi-file download (sequential mode: first file first)")

	for i := 1; i <= len(indices); i++ {
		selectIndices := indices[:i]
		var runTotalSize int64
		for j := range i {
			runTotalSize += sizes[j]
		}

		videoDone, err := d.runOneSequentialBatch(ctx, torrentPath, aria2Cfg, selectIndices,
			runTotalSize, totalSize, progressChan, isVideo[:i])
		if err != nil {
			errChan <- err
			return
		}

		if episodesChan != nil && videoDone > 0 {
			episodesChan <- videoDone
		}

		// Between episodes (not after the last one), wait for the manager to
		// release and re-acquire the semaphore slot so queued downloads get a
		// fair chance to start.
		if i < len(indices) && d.episodeAck != nil {
			select {
			case <-d.episodeAck:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}
	errChan <- nil
}

// sortedFileIndicesByPath returns aria2 file indices (1-based), sizes, and isVideo flags sorted by file path (lexicographic).
// So the first file by name is downloaded first. isVideo[i] is true if the i-th file (in sorted order) is a video file.
func sortedFileIndicesByPath(meta *Meta) (indices []int, sizes []int64, isVideo []bool, totalSize int64) {
	type entry struct {
		aria2Index int // 1-based
		path       string
		size       int64
		video      bool
	}
	var entries []entry
	for i := range meta.Info.Files {
		f := &meta.Info.Files[i]
		path := filepath.Join(meta.Info.Name, filepath.Join(f.Path...))
		entries = append(entries, entry{
			aria2Index: i + 1,
			path:       path,
			size:       f.Length,
			video:      tvcompat.IsVideoFilePath(path),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	indices = make([]int, len(entries))
	sizes = make([]int64, len(entries))
	isVideo = make([]bool, len(entries))
	for i, e := range entries {
		indices[i] = e.aria2Index
		sizes[i] = e.size
		isVideo[i] = e.video
		totalSize += e.size
	}
	return indices, sizes, isVideo, totalSize
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

// parseProgressAndClose parses progress and closes the channel when done.
// This ensures the channel is closed only after all progress updates are sent.
func (d *Aria2Downloader) parseProgressAndClose(r io.Reader, progressChan chan float64) {
	defer close(progressChan)
	d.parseProgress(r, progressChan)
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

func (d *Aria2Downloader) TotalEpisodes() int {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		return 0
	}
	// Only video files count as episodes (posters/subs are not "episodes")
	var n int
	for i := range meta.Info.Files {
		path := filepath.Join(meta.Info.Name, filepath.Join(meta.Info.Files[i].Path...))
		if tvcompat.IsVideoFilePath(path) {
			n++
		}
	}
	return n
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
	logutils.Log.Warn("aria2c did not exit after SIGINT, sending SIGKILL...")
	if err := d.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to send SIGKILL to aria2c process: %w", err)
	}
	logutils.Log.Info("Sent SIGKILL to aria2c process")

	// Wait for process with timeout to avoid infinite blocking
	select {
	case err := <-done:
		// After SIGKILL, any exit error is expected and not a real error
		if err != nil {
			logutils.Log.WithError(err).Debug("aria2c process exited with error after SIGKILL (expected)")
		} else {
			logutils.Log.Debug("aria2c process stopped gracefully after SIGKILL")
		}
		return nil
	case <-time.After(sigkillTimeout):
		logutils.Log.Info("aria2c process did not exit within timeout, considering it stopped")
		return nil
	}
}

// isExpectedExitCode checks if the exit code is expected for a manually stopped process
func (*Aria2Downloader) isExpectedExitCode(exitCode int) bool {
	switch exitCode {
	case aria2ExitUnfinished:
		logutils.Log.Debug("aria2c exited with code for unfinished downloads after manual stop")
		return true
	case signalExitTerm, signalExitKill:
		logutils.Log.Debug("aria2c exited due to signal after manual stop")
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

// GetEarlyTvCompatibility returns a preliminary TV compatibility from torrent metadata (file names)
// so the circle can be shown immediately when the torrent is added.
func (d *Aria2Downloader) GetEarlyTvCompatibility(_ context.Context) (string, error) {
	mainFiles, _, err := d.GetFiles()
	if err != nil {
		return "", err
	}
	return tvcompat.CompatFromTorrentFileNames(mainFiles), nil
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

//nolint:gocyclo // aria2 CLI has many optional flags, each adds a branch
func (d *Aria2Downloader) buildAria2Args(torrentPath string, cfg *config.Aria2Config, selectFileIndices []int) []string {
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

	// File selection: download only these files (1-based indices), in order (for sequential multi-file)
	if len(selectFileIndices) > 0 {
		parts := make([]string, len(selectFileIndices))
		for i, idx := range selectFileIndices {
			parts[i] = strconv.Itoa(idx)
		}
		args = append(args, "--select-file="+strings.Join(parts, ","))
	}

	// Add the torrent file path at the end
	args = append(args, torrentPath)

	logutils.Log.WithField("aria2_args", args).Debug("Built aria2c command arguments")
	return args
}
