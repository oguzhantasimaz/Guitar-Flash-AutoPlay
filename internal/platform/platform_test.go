package platform

import "testing"

func TestParseKey(t *testing.T) {
	for _, n := range []string{"a", "S", " j ", "enter", "Return", "left", ";", "semicolon", "f5", "num3"} {
		if _, err := ParseKey(n); err != nil {
			t.Errorf("ParseKey(%q): %v", n, err)
		}
	}
	if _, err := ParseKey("nope"); err == nil {
		t.Error("ParseKey accepted an unknown key")
	}
}

func TestKeyTableHasNoDuplicateCodes(t *testing.T) {
	win := map[uint16]string{}
	mac := map[uint16]string{}
	for name, c := range keyTable {
		if other, dup := win[c.win]; dup {
			t.Errorf("%q and %q share Windows code %#x", name, other, c.win)
		}
		if other, dup := mac[c.mac]; dup {
			t.Errorf("%q and %q share macOS code %#x", name, other, c.mac)
		}
		win[c.win], mac[c.mac] = name, name
	}
}
