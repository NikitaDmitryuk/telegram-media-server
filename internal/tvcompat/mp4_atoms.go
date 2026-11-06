package tvcompat

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	mp4AtomHeader      = 8
	mp4AtomDepthLimit  = 8
	legacyMP4SizeLimit = int64(math.MaxUint32)
)

// needsTransportStream selects the workaround observed on the legacy LG: a large
// MP4 using 64-bit chunk offsets. This does not identify the decoder's root cause.
func needsTransportStream(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return false, err
	}
	if stat.Size() <= legacyMP4SizeLimit {
		return false, nil
	}
	return containsCo64(file, 0, stat.Size(), 0)
}

func containsCo64(file io.ReaderAt, start, end int64, depth int) (bool, error) {
	if depth > mp4AtomDepthLimit {
		return false, fmt.Errorf("MP4 atom nesting exceeds inspection limit")
	}
	for offset := start; offset < end; {
		size, header, kind, err := readMP4Atom(file, offset, end)
		if err != nil {
			return false, err
		}
		if kind == "co64" {
			return true, nil
		}
		switch kind {
		case "moov", "trak", "mdia", "minf", "stbl":
			found, nestedErr := containsCo64(file, offset+header, offset+size, depth+1)
			if nestedErr != nil || found {
				return found, nestedErr
			}
		}
		offset += size
	}
	return false, nil
}

func readMP4Atom(file io.ReaderAt, offset, end int64) (size, header int64, kind string, err error) {
	var atom [mp4AtomHeader]byte
	if end-offset < mp4AtomHeader {
		return 0, 0, "", fmt.Errorf("truncated MP4 atom header")
	}
	if _, err = file.ReadAt(atom[:], offset); err != nil {
		return 0, 0, "", err
	}
	size = int64(binary.BigEndian.Uint32(atom[:4]))
	header = mp4AtomHeader
	kind = string(atom[4:])
	switch size {
	case 1:
		if end-offset < 2*mp4AtomHeader {
			return 0, 0, "", fmt.Errorf("truncated extended MP4 atom header")
		}
		if _, err = file.ReadAt(atom[:], offset+mp4AtomHeader); err != nil {
			return 0, 0, "", err
		}
		extended := binary.BigEndian.Uint64(atom[:])
		if extended > math.MaxInt64 {
			return 0, 0, "", fmt.Errorf("invalid MP4 atom size")
		}
		size = int64(extended)
		header = 2 * mp4AtomHeader
	case 0:
		size = end - offset
	}
	if size < header || size > end-offset {
		return 0, 0, "", fmt.Errorf("invalid MP4 atom extent")
	}
	return size, header, kind, nil
}
