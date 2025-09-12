package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

const (
	ytdlpTimeout        = 30 * time.Second
	DefaultQuality      = "best[height<=1080]"
	secondsPerMinute    = 60
	gracefulStopTimeout = 5 * time.Second
	forceKillTimeout    = 2 * time.Second
)

type YTDLPDownloader struct {
	bot             *bot.Bot
	url             string
	title           string
	outputFileName  string
	cmd             *exec.Cmd
	cancel          context.CancelFunc
	stoppedManually bool
	config          *tmsconfig.Config
}

func NewYTDLPDownloader(botInstance *bot.Bot, videoURL string, config *tmsconfig.Config) downloader.Downloader {
	videoTitle, err := getVideoTitle(botInstance, videoURL, config)
	if err != nil {
		logutils.Log.WithError(err).Error("Failed to retrieve video title, generating fallback title")
		videoTitle, _ = extractVideoID(videoURL)
		if videoTitle == "" {
			videoTitle = "unknown_video"
		}
	}

	outputFileName := tmsutils.GenerateFileName(videoTitle)
	return &YTDLPDownloader{
		bot:            botInstance,
		url:            videoURL,
		title:          videoTitle,
		outputFileName: outputFileName,
		config:         config,
	}
}

func (d *YTDLPDownloader) StartDownload(ctx context.Context) (progressChan chan float64, errChan chan error, err error) {
	useProxy, err := shouldUseProxy(d.url, d.config)
	if err != nil {
		return nil, nil, fmt.Errorf("error checking proxy requirement: %w", err)
	}

	outputPath := filepath.Join(d.config.MoviePath, d.outputFileName)
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	cmdArgs := d.buildYTDLPArgs(outputPath)

	if useProxy {
		proxy := d.config.Proxy
		logutils.Log.WithField("proxy", proxy).Infof("Using proxy for URL: %s", d.url)
		cmdArgs = append([]string{"--proxy", proxy}, cmdArgs...)
	} else {
		logutils.Log.Infof("No proxy used for URL: %s", d.url)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", cmdArgs...)
	d.cmd = cmd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	progressChan = make(chan float64)
	errChan = make(chan error, 1)

	go d.monitorDownload(ctx, stdout, stderr, progressChan, errChan)

	return progressChan, errChan, nil
}

func (d *YTDLPDownloader) monitorDownload(
	ctx context.Context,
	stdout, stderr io.ReadCloser,
	progressChan chan float64,
	errChan chan error,
) {
	defer close(progressChan)
	errorOutput := make(chan string, 1)

	go func() {
		defer close(errorOutput)
		scanner := bufio.NewScanner(stderr)
		var output strings.Builder
		for scanner.Scan() {
			output.WriteString(scanner.Text() + "\n")
		}
		errorOutput <- output.String()
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "[download]") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				percentStr := strings.TrimSuffix(fields[1], "%")
				if percent, err := strconv.ParseFloat(percentStr, 64); err == nil {
					progressChan <- percent
				}
			}
		}
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- d.cmd.Wait()
	}()

	var processErr error
	select {
	case processErr = <-waitDone:
	case <-ctx.Done():
		logutils.Log.Info("yt-dlp process canceled due to context cancellation")
		if d.cmd.Process != nil {
			if killErr := d.cmd.Process.Kill(); killErr != nil {
				logutils.Log.WithError(killErr).Warn("Failed to kill yt-dlp process")
			}
		}
		processErr = ctx.Err()
	}

	stderrOutput := <-errorOutput

	if processErr != nil {
		if d.stoppedManually || errors.Is(processErr, context.Canceled) || errors.Is(processErr, context.DeadlineExceeded) {
			if errors.Is(processErr, context.DeadlineExceeded) {
				logutils.Log.Info("yt-dlp process timed out")
			} else {
				logutils.Log.Info("yt-dlp process stopped manually")
			}
			errChan <- nil
		} else {
			logutils.Log.WithError(processErr).Errorf("yt-dlp exited with error: %s", stderrOutput)
			detailedErr := fmt.Errorf("yt-dlp failed (exit code: %w):\n%s", processErr, stderrOutput)
			errChan <- detailedErr
		}
	} else {
		errChan <- nil
	}
	close(errChan)
}

func (d *YTDLPDownloader) StopDownload() error {
	d.stoppedManually = true

	if d.cancel != nil {
		logutils.Log.Info("Canceling yt-dlp download context")
		d.cancel()
	}

	if d.cmd != nil && d.cmd.Process != nil {
		logutils.Log.Info("Stopping yt-dlp process gracefully")

		// First try graceful termination (SIGTERM on Unix systems)
		if err := d.cmd.Process.Signal(os.Interrupt); err != nil {
			logutils.Log.WithError(err).Warn("Failed to send interrupt signal, trying kill")
			// If interrupt fails, use kill immediately
			if killErr := d.cmd.Process.Kill(); killErr != nil {
				logutils.Log.WithError(killErr).Warn("Failed to kill yt-dlp process")
				return killErr
			}
		}

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- d.cmd.Wait()
		}()

		select {
		case <-done:
			logutils.Log.Info("yt-dlp process exited gracefully")
		case <-time.After(gracefulStopTimeout):
			// If graceful termination didn't work, force kill
			logutils.Log.Warn("yt-dlp did not exit gracefully, force killing")
			if killErr := d.cmd.Process.Kill(); killErr != nil {
				logutils.Log.WithError(killErr).Warn("Failed to force kill yt-dlp process")
			}

			// Wait a bit more for force kill to take effect
			select {
			case <-done:
				logutils.Log.Info("yt-dlp process exited after force kill")
			case <-time.After(forceKillTimeout):
				logutils.Log.Warn("yt-dlp process did not exit even after force kill, considering it stopped")
			}
		}
	}

	// Additional cleanup: try to remove any remaining temp files
	if err := d.cleanupTempFiles(); err != nil {
		logutils.Log.WithError(err).Warn("Failed to cleanup temporary files after stop")
	}

	logutils.Log.Info("yt-dlp process stopped manually")
	return nil
}

func (d *YTDLPDownloader) GetTitle() (string, error) {
	return d.title, nil
}

func (d *YTDLPDownloader) GetFiles() (mainFiles, tempFiles []string, err error) {
	baseName := strings.TrimSuffix(d.outputFileName, ".mp4")
	mainFiles = []string{d.outputFileName}

	// Comprehensive list of temporary files that yt-dlp can create
	tempFiles = []string{
		// Basic temp files
		baseName + ".part*",
		baseName + ".ytdl",
		baseName + ".ytdlp",
		// Video-specific temp files
		d.outputFileName + ".part*", // e.g., video.mp4.part
		d.outputFileName + ".ytdl",  // e.g., video.mp4.ytdl
		d.outputFileName + ".ytdlp", // e.g., video.mp4.ytdlp
		// Format-specific temp files (yt-dlp uses f-codes for different formats)
		baseName + ".f*.mp4",
		baseName + ".f*.mp4.part*",
		baseName + ".f*.mp4.ytdlp",
		baseName + ".f*.mp4.ytdl",
		baseName + ".f*.webm",
		baseName + ".f*.webm.part*",
		baseName + ".f*.webm.ytdlp",
		baseName + ".f*.webm.ytdl",
		// Additional common patterns
		baseName + ".temp",
		baseName + ".tmp",
		d.outputFileName + ".temp",
		d.outputFileName + ".tmp",
	}
	return mainFiles, tempFiles, nil
}

// cleanupTempFiles attempts to remove temporary files that might be left behind
func (d *YTDLPDownloader) cleanupTempFiles() error {
	if d.config == nil || d.config.MoviePath == "" {
		return nil
	}

	_, tempFiles, err := d.GetFiles()
	if err != nil {
		return err
	}

	var errorList []string
	for _, tempFile := range tempFiles {
		fullPath := filepath.Join(d.config.MoviePath, tempFile)

		// Use glob to handle patterns with *
		if strings.Contains(tempFile, "*") {
			matches, globErr := filepath.Glob(fullPath)
			if globErr != nil {
				continue
			}
			for _, match := range matches {
				if removeErr := os.Remove(match); removeErr != nil && !os.IsNotExist(removeErr) {
					logutils.Log.WithError(removeErr).Debugf("Failed to cleanup temp file: %s", match)
					errorList = append(errorList, removeErr.Error())
				} else if removeErr == nil {
					logutils.Log.Infof("Cleaned up temp file: %s", match)
				}
			}
		} else {
			if removeErr := os.Remove(fullPath); removeErr != nil && !os.IsNotExist(removeErr) {
				logutils.Log.WithError(removeErr).Debugf("Failed to cleanup temp file: %s", fullPath)
				errorList = append(errorList, removeErr.Error())
			} else if removeErr == nil {
				logutils.Log.Infof("Cleaned up temp file: %s", fullPath)
			}
		}
	}

	if len(errorList) > 0 {
		return fmt.Errorf("failed to cleanup some temp files: %s", strings.Join(errorList, "; "))
	}

	return nil
}

func (d *YTDLPDownloader) GetFileSize() (int64, error) {
	useProxy, err := shouldUseProxy(d.url, d.config)
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to determine proxy usage for file size check")
		return 0, nil
	}

	cmdArgs := []string{"--skip-download", "--print-json", d.url}
	if useProxy {
		cmdArgs = append([]string{"--proxy", d.config.Proxy}, cmdArgs...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ytdlpTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp", cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		logutils.Log.WithError(err).Warn("Failed to get video metadata for file size")
		return 0, nil
	}

	var info map[string]any
	if err := json.Unmarshal(output, &info); err != nil {
		logutils.Log.WithError(err).Warn("Failed to parse video metadata JSON")
		return 0, nil
	}

	// Try different filesize fields in order of preference
	sizeFields := []string{
		"filesize",        // Exact file size (if known)
		"filesize_approx", // Approximate file size
		"duration",        // For fallback calculation (duration * estimated bitrate)
	}

	for _, field := range sizeFields {
		if field == "duration" {
			// Fallback: estimate size based on duration
			if duration, ok := info["duration"].(float64); ok && duration > 0 {
				// Estimate ~1MB per minute for standard quality video
				estimatedSize := int64(duration * 1024 * 1024 / secondsPerMinute)
				logutils.Log.WithFields(map[string]any{
					"duration":       duration,
					"estimated_size": estimatedSize,
					"url":            d.url,
				}).Debug("Estimating file size based on duration")
				return estimatedSize, nil
			}
		} else {
			if size, ok := info[field].(float64); ok && size > 0 {
				logutils.Log.WithFields(map[string]any{
					"size_field": field,
					"size":       int64(size),
					"url":        d.url,
				}).Debug("Got file size from video metadata")
				return int64(size), nil
			}
		}
	}

	// If no size information is available, log the available fields for debugging
	logutils.Log.WithFields(map[string]any{
		"available_fields": getMapKeys(info),
		"url":              d.url,
	}).Warn("No file size information available in video metadata")

	return 0, nil
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (d *YTDLPDownloader) StoppedManually() bool {
	return d.stoppedManually
}

func (d *YTDLPDownloader) buildYTDLPArgs(outputPath string) []string {
	videoSettings := d.config.GetVideoSettings()

	args := []string{
		"--newline",
		"-f", videoSettings.QualitySelector,
		"-o", outputPath,
		d.url,
	}

	if videoSettings.CompatibilityMode {
		args = append(args, "-S", "vcodec:h264,acodec:mp3")
		videoSettings.EnableReencoding = true
		videoSettings.VideoCodec = "h264"
		videoSettings.AudioCodec = "mp3"
		videoSettings.OutputFormat = "mp4"
	}

	if videoSettings.EnableReencoding {
		if !videoSettings.ForceReencoding {
			args = append(args, "--recode-video", videoSettings.OutputFormat)
		} else {
			args = append(args, "--recode-video", videoSettings.OutputFormat)
			postprocessorArgs := fmt.Sprintf("ffmpeg:-c:v %s -c:a %s",
				videoSettings.VideoCodec, videoSettings.AudioCodec)

			if videoSettings.FFmpegExtraArgs != "" {
				postprocessorArgs += " " + videoSettings.FFmpegExtraArgs
			}

			args = append(args, "--postprocessor-args", postprocessorArgs)
		}
	}

	return args
}

// Video utility functions

func shouldUseProxy(rawURL string, cfg *tmsconfig.Config) (bool, error) {
	if cfg.Proxy == "" {
		return false, nil
	}

	if cfg.ProxyDomains == "" {
		return true, nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("failed to parse URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	domains := strings.Split(cfg.ProxyDomains, ",")
	for _, domain := range domains {
		if strings.Contains(hostname, strings.TrimSpace(domain)) {
			return true, nil
		}
	}

	return false, nil
}

func getVideoTitle(_ *bot.Bot, videoURL string, cfg *tmsconfig.Config) (string, error) {
	useProxy, err := shouldUseProxy(videoURL, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to determine proxy usage: %w", err)
	}

	args := []string{"--get-title", "--no-playlist"}
	if useProxy && cfg.Proxy != "" {
		args = append(args, "--proxy", cfg.Proxy)
	}
	args = append(args, videoURL)

	ctx, cancel := context.WithTimeout(context.Background(), ytdlpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get video title: %w", err)
	}

	title := strings.TrimSpace(string(output))
	if title == "" {
		return "Unknown Title", nil
	}

	return title, nil
}

func extractVideoID(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	cleanHostname := re.ReplaceAllString(hostname, "_")

	query := parsedURL.Query()
	if v := query.Get("v"); v != "" {
		return fmt.Sprintf("%s_%s", cleanHostname, v), nil
	}

	path := strings.Trim(parsedURL.Path, "/")
	if path != "" {
		cleanPath := re.ReplaceAllString(path, "_")
		return fmt.Sprintf("%s_%s", cleanHostname, cleanPath), nil
	}

	return cleanHostname, nil
}
