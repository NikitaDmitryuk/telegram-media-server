package torrent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/jackpal/bencode-go"
	"github.com/sirupsen/logrus"
)

type TorrentDownloader struct {
	bot             *bot.Bot
	torrentFileName string
	downloadDir     string
	cmd             *exec.Cmd
	rpcPort         string
	progressCtx     context.Context
	progressCancel  context.CancelFunc
	stoppedManually bool
}

type TorrentMeta struct {
	Info struct {
		Name   string `bencode:"name"`
		Length int64  `bencode:"length"`
	} `bencode:"info"`
}

func NewAria2Downloader(bot *bot.Bot, torrentFileName string) *TorrentDownloader {
	return &TorrentDownloader{
		bot:             bot,
		torrentFileName: torrentFileName,
		downloadDir:     tmsconfig.GlobalConfig.MoviePath,
		rpcPort:         "6800",
	}
}

func (d *TorrentDownloader) StartDownload(ctx context.Context) (chan float64, error) {
	logrus.Infof("Starting download for torrent file: %s", d.torrentFileName)

	if err := os.MkdirAll(d.downloadDir, 0755); err != nil {
		logrus.WithError(err).Error("Failed to create download directory")
		return nil, fmt.Errorf("failed to create download directory: %v", err)
	}

	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
	d.cmd = exec.CommandContext(ctx, "aria2c",
		"--dir="+d.downloadDir,
		"--enable-rpc=true",
		"--rpc-listen-port="+d.rpcPort,
		"--seed-time=0",
		torrentPath,
	)

	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		logrus.WithError(err).Error("Failed to capture stdout for aria2c")
		return nil, fmt.Errorf("failed to capture stdout: %v", err)
	}
	stderr, err := d.cmd.StderrPipe()
	if err != nil {
		logrus.WithError(err).Error("Failed to capture stderr for aria2c")
		return nil, fmt.Errorf("failed to capture stderr: %v", err)
	}

	if err := d.cmd.Start(); err != nil {
		logrus.WithError(err).Error("Failed to start aria2c")
		return nil, fmt.Errorf("failed to start aria2c: %v", err)
	}

	progressChan := make(chan float64)

	go d.processOutput(stdout, nil)
	go d.processOutput(stderr, nil)

	d.progressCtx, d.progressCancel = context.WithCancel(ctx)
	go d.trackProgress(d.progressCtx, progressChan)

	go func() {
		err := d.cmd.Wait()
		if err != nil {
			logrus.WithError(err).Error("aria2c process exited with error")
		}
		if d.progressCancel != nil {
			d.progressCancel()
		}
		close(progressChan)
	}()

	return progressChan, nil
}

func (d *TorrentDownloader) StopDownload() error {
	logrus.Info("Stopping torrent download")
	d.stoppedManually = true

	if d.progressCancel != nil {
		d.progressCancel()
	}
	if d.cmd != nil && d.cmd.Process != nil {
		if err := d.cmd.Process.Kill(); err != nil {
			logrus.WithError(err).Error("Failed to kill aria2c process")
			return fmt.Errorf("failed to kill aria2c process: %v", err)
		}
		logrus.Info("aria2c process killed successfully")
	}
	return nil
}

func (d *TorrentDownloader) StoppedManually() bool {
	return d.stoppedManually
}

func (d *TorrentDownloader) GetTitle() (string, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		logrus.WithError(err).Error("Failed to parse torrent meta")
		return "", err
	}
	return meta.Info.Name, nil
}

func (d *TorrentDownloader) GetFiles() (string, []string, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		logrus.WithError(err).Error("Failed to parse torrent meta")
		return "", nil, err
	}
	mainFile := meta.Info.Name
	tempFiles := []string{}

	torrentFile := d.torrentFileName
	tempFiles = append(tempFiles, torrentFile)

	aria2Temp := mainFile + ".aria2"
	tempFiles = append(tempFiles, aria2Temp)

	return mainFile, tempFiles, nil
}

func (d *TorrentDownloader) parseTorrentMeta() (*TorrentMeta, error) {
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
	f, err := os.Open(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open torrent file: %v", err)
	}
	defer f.Close()

	var meta TorrentMeta
	if err := bencode.Unmarshal(f, &meta); err != nil {
		return nil, fmt.Errorf("failed to decode torrent meta: %v", err)
	}
	if meta.Info.Name == "" {
		return nil, fmt.Errorf("torrent meta does not contain a file name")
	}
	return &meta, nil
}

func (d *TorrentDownloader) processOutput(pipe io.ReadCloser, progressChan chan float64) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		logrus.Debugf("aria2c: %s", line)
	}
	if err := scanner.Err(); err != nil {
		logrus.WithError(err).Error("Error reading aria2c output")
	}
}

func (d *TorrentDownloader) trackProgress(ctx context.Context, progressChan chan float64) {
	client := &http.Client{}
	url := fmt.Sprintf("http://localhost:%s/jsonrpc", d.rpcPort)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			requestBody := `{"jsonrpc": "2.0", "method": "aria2.tellActive", "id": "1"}`
			req, err := http.NewRequest("POST", url, strings.NewReader(requestBody))
			if err != nil {
				logrus.WithError(err).Error("Failed to create RPC request")
				time.Sleep(1 * time.Second)
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				logrus.WithError(err).Error("Failed to send RPC request")
				time.Sleep(1 * time.Second)
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				logrus.WithError(err).Error("Failed to read RPC response")
				time.Sleep(1 * time.Second)
				continue
			}

			var rpcResponse struct {
				Result []struct {
					CompletedLength string `json:"completedLength"`
					TotalLength     string `json:"totalLength"`
				} `json:"result"`
			}

			err = json.Unmarshal(body, &rpcResponse)
			if err != nil {
				logrus.WithError(err).Error("Failed to parse RPC response")
				time.Sleep(1 * time.Second)
				continue
			}

			if len(rpcResponse.Result) > 0 {
				completed, err1 := strconv.ParseFloat(rpcResponse.Result[0].CompletedLength, 64)
				total, err2 := strconv.ParseFloat(rpcResponse.Result[0].TotalLength, 64)
				if err1 == nil && err2 == nil && total > 0 {
					progress := (completed / total) * 100
					progressChan <- progress
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (d *TorrentDownloader) GetFileSize() (int64, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		logrus.WithError(err).Error("Failed to parse torrent meta")
		return 0, err
	}

	if meta.Info.Length == 0 {
		return 0, fmt.Errorf("torrent meta does not contain file length")
	}
	return meta.Info.Length, nil
}
