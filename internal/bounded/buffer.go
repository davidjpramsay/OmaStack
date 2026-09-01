package bounded

import "bytes"

// Buffer consumes every byte written while retaining at most Limit bytes.
// Returning the original write length prevents subprocess pipes from failing
// merely because diagnostic output exceeded the in-memory capture budget.
type Buffer struct {
	buffer    bytes.Buffer
	Limit     int
	Truncated bool
}

func NewBuffer(limit int) *Buffer {
	if limit < 0 {
		limit = 0
	}
	return &Buffer{Limit: limit}
}

func (b *Buffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := b.Limit - b.buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buffer.Write(value)
	}
	if original > remaining {
		b.Truncated = true
	}
	return original, nil
}

func (b *Buffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *Buffer) String() string { return b.buffer.String() }
func (b *Buffer) Len() int       { return b.buffer.Len() }
