package aria2

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/bot"
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/downloader"
	"github.com/jackpal/bencode-go"
	"github.com/sirupsen/logrus"
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
}

func NewAria2Downloader(bot *bot.Bot, torrentFileName string) downloader.Downloader {
	return &Aria2Downloader{
		bot:             bot,
		torrentFileName: torrentFileName,
		downloadDir:     tmsconfig.GlobalConfig.MoviePath,
	}
}

func (d *Aria2Downloader) StartDownload(ctx context.Context) (chan float64, chan error, error) {
	dhtPort := 6881 + rand.Intn(1000)
	listenPort := 7881 + rand.Intn(1000)

	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
	d.cmd = exec.CommandContext(ctx, "aria2c",
		"--dir", d.downloadDir,
		"--seed-time=0",
		"--summary-interval=3",
		"--enable-dht=true",
		fmt.Sprintf("--dht-listen-port=%d", dhtPort),
		fmt.Sprintf("--listen-port=%d", listenPort),
		"--bt-tracker=udp://tracker.openbittorrent.com:80,udp://tracker.opentrackr.org:1337,udp://tracker.leechers-paradise.org:6969",
		"--bt-exclude-tracker=http://retracker.local",
		"--disable-ipv6",
		"--max-connection-per-server=16",
		"--split=16",
		"--min-split-size=1M",
		torrentPath)

	stdout, err := d.cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture stdout: %v", err)
	}

	if err := d.cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start aria2c: %v", err)
	}

	progressChan := make(chan float64)
	errChan := make(chan error, 1)

	go func() {
		defer close(progressChan)
		defer close(errChan)

		d.parseProgress(stdout, progressChan)

		err := d.cmd.Wait()
		if err != nil && !d.stoppedManually {
			logrus.Warnf("aria2c exited with error: %v", err)
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	return progressChan, errChan, nil
}

func (d *Aria2Downloader) parseProgress(r io.Reader, progressChan chan float64) {
	reProgress := regexp.MustCompile(`\(\s*(\d+\.?\d*)%\s*\)`)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		logrus.Debugf("aria2c output: %s", line)

		matches := reProgress.FindStringSubmatch(line)
		if len(matches) > 1 {
			if prog, err := strconv.ParseFloat(matches[1], 64); err == nil {
				progressChan <- prog
			} else {
				logrus.Errorf("failed to parse progress value: %v", err)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
	if err := scanner.Err(); err != nil {
		logrus.Errorf("error reading aria2c output: %v", err)
	}
}

func (d *Aria2Downloader) StopDownload() error {
	d.stoppedManually = true
	if d.cmd != nil && d.cmd.Process != nil {
		if err := d.cmd.Process.Signal(os.Interrupt); err != nil {
			return fmt.Errorf("failed to send SIGINT to aria2c process: %v", err)
		}
		logrus.Info("Sent SIGINT to aria2c process")
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

func (d *Aria2Downloader) GetFiles() ([]string, []string, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		return nil, nil, err
	}

	var mainFiles []string
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

	tempFiles := []string{
		filepath.Join(meta.Info.Name + ".aria2"),
		filepath.Join(d.torrentFileName),
	}

	return mainFiles, tempFiles, nil
}

func (d *Aria2Downloader) GetFileSize() (int64, error) {
	meta, err := d.parseTorrentMeta()
	if err != nil {
		logrus.WithError(err).Warn("Failed to parse torrent metadata for file size, returning 0")
		return 0, nil
	}
	if meta.Info.Length == 0 {
		logrus.Warn("Torrent metadata does not indicate file size, returning 0")
		return 0, nil
	}
	return meta.Info.Length, nil
}

func (d *Aria2Downloader) parseTorrentMeta() (*TorrentMeta, error) {
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
