package bounded

import "testing"

func TestBufferConsumesInputAndCapsStorage(t *testing.T) {
	buffer := NewBuffer(5)
	if written, err := buffer.Write([]byte("abcdefgh")); err != nil || written != 8 {
		t.Fatalf("write = %d, %v", written, err)
	}
	if buffer.String() != "abcde" || buffer.Len() != 5 || !buffer.Truncated {
		t.Fatalf("buffer = %q len=%d truncated=%v", buffer.String(), buffer.Len(), buffer.Truncated)
	}
}
