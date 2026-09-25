//go:build windows

package platform

import (
	"testing"
	"unsafe"
)

func TestInputStructMatchesWin32(t *testing.T) {
	// sizeof(INPUT) is 40 bytes on 64-bit Windows and 28 on 32-bit.
	want := uintptr(40)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 28
	}
	if got := unsafe.Sizeof(input{}); got != want {
		t.Fatalf("sizeof(input) = %d, want %d", got, want)
	}
}
