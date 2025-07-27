package aria2

import (
	"bufio"
	"context"
	"crypto/rand"
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
	tmsconfig "github.com/NikitaDmitryuk/telegram-media-server/internal/config"
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
}

func NewAria2Downloader(botInstance *bot.Bot, torrentFileName string) downloader.Downloader {
	return &Aria2Downloader{
		bot:             botInstance,
		torrentFileName: torrentFileName,
		downloadDir:     tmsconfig.GlobalConfig.MoviePath,
	}
}

func (d *Aria2Downloader) StartDownload(ctx context.Context) (progressChan chan float64, errChan chan error, err error) {
	const dhtPortBase, listenPortBase, portRange = 6881, 7881, 1000

	dhtPort, err := generateRandomPort(dhtPortBase, portRange)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate DHT port: %w", err)
	}

	listenPort, err := generateRandomPort(listenPortBase, portRange)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate listen port: %w", err)
	}

	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
	cmdArgs := []string{
		"--dir", d.downloadDir,
		"--seed-time=0",
		"--summary-interval=3",
		"--enable-dht=true",
		"--bt-enable-lpd=true",
		fmt.Sprintf("--dht-listen-port=%d", dhtPort),
		fmt.Sprintf("--listen-port=%d", listenPort),
		"--max-connection-per-server=16",
		"--split=16",
		"--min-split-size=1M",
		torrentPath,
	}
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

		d.parseProgress(stdout, progressChan)

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

		// Ждём до 3 секунд завершения процесса
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
	if meta.Info.Length == 0 {
		logutils.Log.Warn("Torrent metadata does not indicate file size, returning 0")
		return 0, nil
	}
	return meta.Info.Length, nil
}

func (d *Aria2Downloader) parseTorrentMeta() (*TorrentMeta, error) {
	torrentPath := filepath.Join(d.downloadDir, d.torrentFileName)
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

func generateRandomPort(base, rangeSize int) (int, error) {
	if rangeSize <= 0 {
		return 0, fmt.Errorf("invalid range size: %d", rangeSize)
	}

	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return 0, fmt.Errorf("failed to generate random port: %w", err)
	}
	return base + int(b[0])%rangeSize, nil
}
