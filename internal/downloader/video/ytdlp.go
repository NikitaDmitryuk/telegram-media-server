package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	ytdlpTimeout   = 30 * time.Second
	DefaultQuality = "best[height<=1080]"
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
	if d.cancel != nil {
		d.cancel()
	}
	d.stoppedManually = true
	logutils.Log.Info("Download process canceled")
	return nil
}

func (d *YTDLPDownloader) GetTitle() (string, error) {
	return d.title, nil
}

func (d *YTDLPDownloader) GetFiles() (mainFiles, tempFiles []string, err error) {
	baseName := strings.TrimSuffix(d.outputFileName, ".mp4")
	mainFiles = []string{d.outputFileName}
	tempFiles = []string{
		baseName + ".part*",
		baseName + ".ytdl",
		baseName + ".f*.mp4",
		baseName + ".f*.mp4.part*",
		baseName + ".f*.mp4.ytdlp",
		baseName + ".f*.mp4.ytdl",
	}
	return mainFiles, tempFiles, nil
}

func (d *YTDLPDownloader) GetFileSize() (int64, error) {
	useProxy, err := shouldUseProxy(d.url, d.config)
	if err != nil {
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
		return 0, nil
	}

	var info map[string]any
	if err := json.Unmarshal(output, &info); err != nil {
		return 0, nil
	}

	if size, ok := info["filesize"].(float64); ok {
		return int64(size), nil
	}
	if size, ok := info["filesize_approx"].(float64); ok {
		return int64(size), nil
	}

	return 0, nil
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
