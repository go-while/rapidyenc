//go:build linux || darwin

package rapidyenc

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/debug"
	"syscall"
	"testing"
	"unsafe"
)

// zzGuard is four mmap'd pages: src page, PROT_NONE, dst page, PROT_NONE.
// src is placed so its last byte is the last byte before the first guard,
// and dst gets exactly len(src) bytes ending at the second guard. Any read
// past src[len(src)-1] or write past dst[len(src)-1] faults.
type zzGuard struct {
	mem []byte
	ps  int
}

func newZZGuard(t *testing.T) *zzGuard {
	ps := os.Getpagesize()
	mem, err := syscall.Mmap(-1, 0, 4*ps, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{1, 3} {
		if err := syscall.Mprotect(mem[p*ps:(p+1)*ps], syscall.PROT_NONE); err != nil {
			t.Fatal(err)
		}
	}
	return &zzGuard{mem, ps}
}

func (g *zzGuard) close() { syscall.Munmap(g.mem) }

type zzFault struct {
	msg  string
	addr string
}

// decode runs decodeIncremental on src copied to the end of the src page,
// with dst ending at the second guard, and catches faults.
func (g *zzGuard) decode(src []byte, st State, dstLen int) (r zzRes, f *zzFault) {
	ps := g.ps
	s := g.mem[ps-len(src) : ps : ps]
	copy(s, src)
	d := g.mem[3*ps-dstLen : 3*ps : 3*ps]
	for i := range d {
		d[i] = 0xEE
	}
	defer func() {
		if e := recover(); e != nil {
			f = &zzFault{msg: fmt.Sprint(e)}
			if ae, ok := e.(interface{ Addr() uintptr }); ok {
				a := ae.Addr()
				sEnd := uintptr(unsafe.Pointer(&g.mem[0])) + uintptr(ps)
				dEnd := uintptr(unsafe.Pointer(&g.mem[0])) + uintptr(3*ps)
				switch {
				case a >= sEnd && a < sEnd+uintptr(ps):
					f.addr = fmt.Sprintf("READ %d bytes past the end of src", a-sEnd+1)
				case a >= dEnd:
					f.addr = fmt.Sprintf("WRITE %d bytes past the end of dst", a-dEnd+1)
				default:
					f.addr = fmt.Sprintf("fault address %#x", a)
				}
			}
		}
	}()
	old := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(old)
	n, out, ps2, end, _ := decodeIncremental(d, s, st)
	return zzRes{n, append([]byte(nil), out...), int(ps2), int(end)}, nil
}

// TestZZGuard runs the boundary inputs (all lengths, all listed states) and
// pseudo-random inputs of up to 4 KiB (ZZ_GUARD_N of them) through
// decodeIncremental with src and dst ending at PROT_NONE pages, and checks
// the result against decodeGeneric on ordinary memory.
func TestZZGuard(t *testing.T) {
	runtime.LockOSThread() // SetPanicOnFault is per goroutine; keep it simple
	defer runtime.UnlockOSThread()
	g := newZZGuard(t)
	defer g.close()
	t.Logf("kernel %s, page size %d", DecodeKernel(), g.ps)

	n, faults, differ := 0, 0, 0
	check := func(src []byte, st State, desc string) {
		if len(src) > g.ps {
			return
		}
		n++
		got, f := g.decode(src, st, len(src))
		if f != nil {
			faults++
			if faults <= 20 {
				t.Errorf("FAULT len %d state %d %s: %s (%s)\n  src %q", len(src), st, desc, f.addr, f.msg, src)
			}
			return
		}
		want := zzTheirs(decodeGeneric, src, st)
		if d := got.diff(want); d != "" {
			differ++
			if differ <= 20 {
				t.Errorf("guarded kernel differs from generic (%s) len %d state %d %s\n  kernel n=%d st=%d end=%d out %q\n  gen    n=%d st=%d end=%d out %q\n  src %q",
					d, len(src), st, desc, got.n, got.state, got.end, got.out, want.n, want.state, want.end, want.out, src)
			}
		}
		if !bytes.Equal(got.out, g.mem[3*g.ps-len(src):][:len(got.out)]) {
			t.Errorf("decoded slice is not at the start of dst, len %d state %d %s", len(src), st, desc)
		}
	}
	zzBoundaryInputs(func(src, pat []byte, off, tail int, states []State) {
		for _, st := range states {
			check(src, st, fmt.Sprintf("pat %q off %d tail %d", pat, off, tail))
		}
	})
	nb := n
	seed := zzEnvInt("ZZ_SEED", 1)
	r := rand.New(rand.NewPCG(uint64(seed), 0x243f6a8885a308d3))
	for range zzEnvInt("ZZ_GUARD_N", 200000) {
		src := zzRandomInput(r, 4096)
		check(src, zzAllStates[r.IntN(len(zzAllStates))], "random")
	}
	t.Logf("%d inputs (%d boundary, %d random, seed %d): %d faults, %d differ from generic", n, nb, n-nb, seed, faults, differ)
}

var zzSink byte

// TestZZGuardSelfTest makes sure the guard pages really fault and that the
// fault is caught, so a clean TestZZGuard means something.
func TestZZGuardSelfTest(t *testing.T) {
	g := newZZGuard(t)
	defer g.close()
	caught := func(f func()) (ok bool) {
		defer func() { ok = recover() != nil }()
		old := debug.SetPanicOnFault(true)
		defer debug.SetPanicOnFault(old)
		f()
		return false
	}
	p := unsafe.Pointer(&g.mem[g.ps-1])
	if !caught(func() { zzSink = *(*byte)(unsafe.Add(p, 1)) }) {
		t.Fatal("read of the src guard page did not fault")
	}
	q := unsafe.Pointer(&g.mem[3*g.ps-1])
	if !caught(func() { *(*byte)(unsafe.Add(q, 1)) = 1 }) {
		t.Fatal("write to the dst guard page did not fault")
	}
	if caught(func() { zzSink = *(*byte)(p); *(*byte)(q) = 1 }) {
		t.Fatal("access to the last accessible bytes faulted")
	}
	t.Logf("page size %d: guard pages fault and are caught", g.ps)
}
