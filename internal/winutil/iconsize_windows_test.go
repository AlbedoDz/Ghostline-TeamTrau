package winutil

import "testing"

func TestSmallIconSize(t *testing.T) {
	if n := SmallIconSize(); n < 16 || n > 64 {
		t.Fatalf("SmallIconSize = %d", n)
	}
}
