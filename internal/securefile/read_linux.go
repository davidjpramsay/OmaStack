//go:build linux

package securefile

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// ReadRegular opens the final path component with O_NOFOLLOW, verifies the
// opened object rather than a pre-open pathname, and enforces a byte limit.
func ReadRegular(path string, maxBytes int64, requirePrivate bool) ([]byte, error) {
	// Reject FIFOs/devices after opening without waiting for a FIFO writer.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("open %s: invalid file descriptor", path)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if requirePrivate && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s must not allow group or other access", path)
	}
	if maxBytes < 1 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxBytes)
	}
	return data, nil
}
