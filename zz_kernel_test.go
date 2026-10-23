package rapidyenc

import (
	"os"
	"runtime"
	"testing"
)

// TestZZKernel logs the decode kernel and fails unless it is the one named
// in ZZ_WANT_KERNEL (e.g. "NEON"), so the later steps cannot silently test
// the C library or the generic fallback.
func TestZZKernel(t *testing.T) {
	t.Logf("GOOS=%s GOARCH=%s DecodeKernel()=%q EncodeKernel()=%q", runtime.GOOS, runtime.GOARCH, DecodeKernel(), EncodeKernel())
	if want := os.Getenv("ZZ_WANT_KERNEL"); want != "" && DecodeKernel() != want {
		t.Fatalf("DecodeKernel() = %q, want %q", DecodeKernel(), want)
	}
}
