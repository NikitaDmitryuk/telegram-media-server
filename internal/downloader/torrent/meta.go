package aria2

import (
	"fmt"
	"os"

	"github.com/jackpal/bencode-go"
)

type Meta struct {
	Info struct {
		Name   string `bencode:"name"`
		Length int64  `bencode:"length"`
		Files  []struct {
			Length int64    `bencode:"length"`
			Path   []string `bencode:"path"`
		} `bencode:"files"`
	} `bencode:"info"`
}

func ParseMeta(filePath string) (*Meta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Printf("Failed to close torrent file: %v\n", cerr)
		}
	}()

	var meta Meta
	if err := bencode.Unmarshal(f, &meta); err != nil {
		return nil, fmt.Errorf("failed to decode torrent meta: %w", err)
	}
	if meta.Info.Name == "" {
		return nil, fmt.Errorf("torrent meta does not contain a file name")
	}
	return &meta, nil
}
