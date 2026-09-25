package ui

import "testing"

func TestOffsetText(t *testing.T) {
	for ms, want := range map[int]string{0: "on time", 30: "+30 ms later", -90: "90 ms earlier"} {
		if got := offsetText(ms); got != want {
			t.Errorf("offsetText(%d) = %q, want %q", ms, got, want)
		}
	}
}
