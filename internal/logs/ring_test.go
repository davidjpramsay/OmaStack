package logs

import (
	"reflect"
	"testing"
)

func TestRingTruncatesAndClears(t *testing.T) {
	ring := NewRing[int](3)
	for value := 1; value <= 5; value++ {
		ring.Add(value)
	}
	if got := ring.Values(); !reflect.DeepEqual(got, []int{3, 4, 5}) {
		t.Fatalf("values=%v", got)
	}
	ring.Clear()
	if len(ring.Values()) != 0 {
		t.Fatal("clear retained values")
	}
}
