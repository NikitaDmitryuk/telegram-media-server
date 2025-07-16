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
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/logutils"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
)

type YTDLPDownloader struct {
	bot             *bot.Bot
	url             string
	title           string
	outputFileName  string
	cmd             *exec.Cmd
	cancel          context.CancelFunc
	stoppedManually bool
}

func NewYTDLPDownloader(botInstance *bot.Bot, videoURL string) downloader.Downloader {
	videoTitle, err := getVideoTitle(botInstance, videoURL)
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
	}
}

func (d *YTDLPDownloader) StartDownload(ctx context.Context) (progressChan chan float64, errChan chan error, err error) {
	useProxy, err := shouldUseProxy(d.bot, d.url)
	if err != nil {
		return nil, nil, fmt.Errorf("error checking proxy requirement: %w", err)
	}

	outputPath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, d.outputFileName)
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	cmdArgs := []string{
		"--newline",
		"-S", "vcodec:h264,acodec:mp3",
		"-f", "bv*+ba/b",
		"--recode-video", "mp4",
		"--postprocessor-args", "ffmpeg:-pix_fmt yuv420p",
		"-o", outputPath, d.url,
	}

	if useProxy {
		proxy := tmsconfig.GlobalConfig.Proxy
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

	go d.monitorDownload(stdout, stderr, progressChan, errChan)

	return progressChan, errChan, nil
}

func (d *YTDLPDownloader) monitorDownload(stdout, stderr io.ReadCloser, progressChan chan float64, errChan chan error) {
	defer close(progressChan)
	errorOutput := make(chan string)

	go func() {
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

	if err := d.cmd.Wait(); err != nil {
		stderrOutput := <-errorOutput
		logutils.Log.WithError(err).Errorf("yt-dlp exited with error: %s", stderrOutput)
		errChan <- err
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
	mainFiles = []string{d.outputFileName}
	tempFiles = []string{
		d.outputFileName + ".part",
		d.outputFileName + ".ytdl",
	}

	pattern := filepath.Join(tmsconfig.GlobalConfig.MoviePath, d.outputFileName+".part-Frag*")
	matchedFiles, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search files with pattern %s: %w", pattern, err)
	}

	tempFiles = append(tempFiles, matchedFiles...)
	return mainFiles, tempFiles, nil
}

func (d *YTDLPDownloader) GetFileSize() (int64, error) {
	useProxy, err := shouldUseProxy(d.bot, d.url)
	if err != nil {
		return 0, nil
	}

	cmdArgs := []string{"--skip-download", "--print-json", d.url}
	if useProxy {
		cmdArgs = append([]string{"--proxy", tmsconfig.GlobalConfig.Proxy}, cmdArgs...)
	}

	cmd := exec.Command("yt-dlp", cmdArgs...)
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

func shouldUseProxy(_ *bot.Bot, rawURL string) (bool, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("invalid URL: %w", err)
	}

	proxy := tmsconfig.GlobalConfig.Proxy
	if proxy == "" {
		return false, nil
	}

	targetHosts := tmsconfig.GlobalConfig.ProxyHost
	if targetHosts == "" {
		return true, nil
	}

	for _, host := range strings.Split(targetHosts, ",") {
		if parsedURL.Host == strings.TrimSpace(host) {
			return true, nil
		}
	}

	return false, nil
}

func getVideoTitle(botInstance *bot.Bot, videoURL string) (string, error) {
	useProxy, err := shouldUseProxy(botInstance, videoURL)
	if err != nil {
		return "", fmt.Errorf("error checking proxy requirement: %w", err)
	}

	cmdArgs := []string{"--get-title", videoURL}
	if useProxy {
		cmdArgs = append([]string{"--proxy", tmsconfig.GlobalConfig.Proxy}, cmdArgs...)
	}

	cmd := exec.Command("yt-dlp", cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp failed: %w", err)
	}

	videoTitle := strings.TrimSpace(string(output))
	if videoTitle == "" {
		videoID, err := extractVideoID(videoURL)
		if err != nil {
			return "unknown_video", nil
		}
		return videoID, nil
	}

	return videoTitle, nil
}

func extractVideoID(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	queryParams := parsedURL.Query()
	if videoID := queryParams.Get("v"); videoID != "" {
		return videoID, nil
	}

	pathSegments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathSegments) > 0 {
		return pathSegments[len(pathSegments)-1], nil
	}

	return "", errors.New("unable to extract video ID")
}
