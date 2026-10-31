package scanner

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hashChunk is the size read from the start and from the end of a file.
const hashChunk = 64 << 10

// PartialHash identifies a file by its first and last 64 KB plus its size,
// so a renamed or moved file can be recognized without reading all of it.
// The file is opened read-only and never modified.
func PartialHash(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("partial hash %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("partial hash %s: %w", path, err)
	}
	size := info.Size()

	h := sha256.New()
	var sz [8]byte
	binary.LittleEndian.PutUint64(sz[:], uint64(size)) //nolint:gosec // size is never negative
	h.Write(sz[:])

	buf := make([]byte, hashChunk)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("partial hash %s: %w", path, err)
	}
	h.Write(buf[:n])

	if size > 2*hashChunk {
		if _, err := f.ReadAt(buf, size-hashChunk); err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("partial hash %s: %w", path, err)
		}
		h.Write(buf)
	} else if size > hashChunk {
		rest, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("partial hash %s: %w", path, err)
		}
		h.Write(rest)
	}
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
}
