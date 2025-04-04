package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	tmsutils "github.com/NikitaDmitryuk/telegram-media-server/internal/utils"
	"github.com/sirupsen/logrus"
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

func NewYTDLPDownloader(bot *bot.Bot, url string) downloader.Downloader {
	videoTitle, err := getVideoTitle(bot, url)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve video title, generating fallback title")
		videoTitle, _ = extractVideoID(url)
		if videoTitle == "" {
			videoTitle = "unknown_video"
		}
	}

	outputFileName := tmsutils.GenerateFileName(videoTitle)
	return &YTDLPDownloader{
		bot:            bot,
		url:            url,
		title:          videoTitle,
		outputFileName: outputFileName,
	}
}

func (d *YTDLPDownloader) StartDownload(ctx context.Context) (chan float64, chan error, error) {
	useProxy, err := shouldUseProxy(d.bot, d.url)
	if err != nil {
		return nil, nil, fmt.Errorf("error checking proxy requirement: %v", err)
	}

	outputFileName := tmsutils.GenerateFileName(d.title)
	d.outputFileName = outputFileName
	outputPath := filepath.Join(tmsconfig.GlobalConfig.MoviePath, outputFileName)

	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	var cmd *exec.Cmd
	if useProxy {
		proxy := tmsconfig.GlobalConfig.Proxy
		logrus.WithField("proxy", proxy).Infof("Using proxy for URL: %s", d.url)
		cmd = exec.CommandContext(ctx, "yt-dlp", "--newline", "--proxy", proxy,
			"-S", "vext:mp4,aext:m4a",
			"-f", "bv*+ba/b",
			"--recode-video", "mp4",
			"-o", outputPath, d.url)
	} else {
		logrus.Infof("No proxy used for URL: %s", d.url)
		cmd = exec.CommandContext(ctx, "yt-dlp", "--newline",
			"-S", "vext:mp4,aext:m4a",
			"-f", "bv*+ba/b",
			"--recode-video", "mp4",
			"-o", outputPath, d.url)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logrus.WithError(err).Error("Failed to create stdout pipe")
		return nil, nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logrus.WithError(err).Error("Failed to create stderr pipe")
		return nil, nil, fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		logrus.WithError(err).Error("Failed to start yt-dlp")
		return nil, nil, fmt.Errorf("failed to start yt-dlp: %v", err)
	}

	d.cmd = cmd
	progressChan := make(chan float64)
	errChan := make(chan error, 1)

	go func() {
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

		if err := cmd.Wait(); err != nil {
			stderrOutput := <-errorOutput
			logrus.WithError(err).Errorf("yt-dlp exited with error: %s", stderrOutput)
			errChan <- err
		} else {
			errChan <- nil
		}
		close(errChan)
	}()

	return progressChan, errChan, nil
}

func (d *YTDLPDownloader) StopDownload() error {
	if d.cancel != nil {
		d.cancel()
	}
	d.stoppedManually = true
	logrus.Info("Download process canceled")
	return nil
}

func (d *YTDLPDownloader) GetTitle() (string, error) {
	return d.title, nil
}

func (d *YTDLPDownloader) GetFiles() (string, []string, error) {
	mainFile := d.outputFileName
	tempFilePart := d.outputFileName + ".part"
	tempfileYtdl := d.outputFileName + ".ytdl"

	tempFiles := []string{tempFilePart, tempfileYtdl}

	pattern := d.outputFileName + ".part-Frag*"
	matchedFiles, err := filepath.Glob(pattern)
	if err != nil {
		return "", nil, fmt.Errorf("failed to search files with pattern %s: %w", pattern, err)
	}

	tempFiles = append(tempFiles, matchedFiles...)

	return mainFile, tempFiles, nil
}

func (d *YTDLPDownloader) GetFileSize() (int64, error) {
	useProxy, err := shouldUseProxy(d.bot, d.url)
	if err != nil {
		logrus.Warnf("Error checking proxy requirement: %v; returning 0", err)
		return 0, nil
	}

	var cmd *exec.Cmd
	if useProxy {
		proxy := tmsconfig.GlobalConfig.Proxy
		cmd = exec.Command("yt-dlp", "--proxy", proxy, "--skip-download", "--print-json", d.url)
	} else {
		cmd = exec.Command("yt-dlp", "--skip-download", "--print-json", d.url)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logrus.WithError(err).Error("Failed to create stderr pipe")
		return 0, nil
	}

	if err := cmd.Start(); err != nil {
		logrus.WithError(err).Error("Failed to start yt-dlp")
		return 0, nil
	}

	errorOutput := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stderr)
		var output strings.Builder
		for scanner.Scan() {
			output.WriteString(scanner.Text() + "\n")
		}
		errorOutput <- output.String()
	}()

	output, err := cmd.Output()
	if err != nil {
		stderrOutput := <-errorOutput
		logrus.WithError(err).Errorf("yt-dlp failed with error: %s", stderrOutput)
		return 0, nil
	}

	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		logrus.Warnf("Failed to parse metadata JSON: %v; returning 0", err)
		return 0, nil
	}

	var size float64
	if v, ok := info["filesize"]; ok && v != nil {
		if filesize, ok := v.(float64); ok {
			size = filesize
		} else {
			logrus.Warn("Unexpected type for filesize; returning 0")
			return 0, nil
		}
	} else if v, ok := info["filesize_approx"]; ok && v != nil {
		if filesizeApprox, ok := v.(float64); ok {
			size = filesizeApprox
		} else {
			logrus.Warn("Unexpected type for filesize_approx; returning 0")
			return 0, nil
		}
	} else {
		logrus.Warn("Filesize information not found in metadata; returning 0")
		return 0, nil
	}

	return int64(size), nil
}

func (d *YTDLPDownloader) StoppedManually() bool {
	return d.stoppedManually
}

func shouldUseProxy(bot *bot.Bot, rawURL string) (bool, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		logrus.WithError(err).Error("Invalid URL")
		return false, errors.New("invalid URL")
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

func getVideoTitle(bot *bot.Bot, url string) (string, error) {
	useProxy, err := shouldUseProxy(bot, url)
	if err != nil {
		logrus.WithError(err).Error("Error checking proxy requirement")
		return "", fmt.Errorf("error checking proxy requirement: %v", err)
	}

	var cmd *exec.Cmd
	if useProxy {
		proxy := tmsconfig.GlobalConfig.Proxy
		cmd = exec.Command("yt-dlp", "--proxy", proxy, "--get-title", url)
	} else {
		cmd = exec.Command("yt-dlp", "--get-title", url)
	}

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		logrus.WithError(err).Errorf("yt-dlp failed with error: %s", stderrBuf.String())
		return "", fmt.Errorf("yt-dlp failed: %v", err)
	}

	videoTitle := strings.TrimSpace(stdoutBuf.String())
	if videoTitle == "" {
		logrus.Warn("Video title is empty, falling back to video ID")
		videoID, idErr := extractVideoID(url)
		if idErr != nil {
			logrus.WithError(idErr).Error("Failed to extract video ID")
			return "unknown_video", nil
		}
		return videoID, nil
	}

	logrus.Infof("Video title retrieved: %s", videoTitle)
	return videoTitle, nil
}

func extractVideoID(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %v", err)
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
