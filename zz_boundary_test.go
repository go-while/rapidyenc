package rapidyenc

import (
	"bytes"
	"testing"
	"unsafe"
)

// TestZZBoundaries is go-yenc-simd's TestBoundaries for this package: every
// string of up to 4 of the bytes "=\r\n.ya" at every offset 0..140 of plain
// data, with tails of 0, 3, 24, 47, 70 and 140 bytes, at six alignments,
// decoded by decodeIncremental (SIMD or C) and by decodeGeneric.
func TestZZBoundaries(t *testing.T) {
	var pats [][]byte
	var gen func(p []byte)
	gen = func(p []byte) {
		if len(p) > 0 {
			pats = append(pats, append([]byte(nil), p...))
		}
		if len(p) < 4 {
			for _, c := range []byte("=\r\n.ya") {
				gen(append(p, c))
			}
		}
	}
	gen(nil)
	plain := make([]byte, 300)
	for i := range plain {
		plain[i] = 'A' + byte(i%26)
	}
	all := []State{StateCRLF, StateEQ, StateCR, StateNone, StateCRLFDT, StateCRLFDTCR, StateCRLFEQ}
	buf := make([]byte, 64+400+128)
	dst1 := make([]byte, 400+64)
	dst2 := make([]byte, 400+64)
	n, bad := 0, 0
	for _, pat := range pats {
		for off := 0; off <= 140; off++ {
			for _, tail := range []int{0, 3, 24, 47, 70, 140} {
				src := append(append(append([]byte(nil), plain[:off]...), pat...), plain[off:off+tail]...)
				if len(src) <= 128 {
					continue // decodeSIMD hands these to decodeGeneric
				}
				states := []State{StateNone, StateCRLF}
				if off < 4 {
					states = all
				}
				for _, align := range []int{0, 1, 15, 31, 47, 63} {
					base := int(uintptr(unsafe.Pointer(&buf[0])) & 63)
					shift := (align - base + 64) & 63
					s := buf[shift : shift+len(src)]
					copy(s, src)
					for _, st := range states {
						n++
						n1, d1, s1, e1, err1 := decodeIncremental(dst1, s, st)
						n2, d2, s2, e2, err2 := decodeGeneric(dst2, src, st)
						if n1 != n2 || !bytes.Equal(d1, d2) || s1 != s2 || e1 != e2 || (err1 == nil) != (err2 == nil) {
							bad++
							if bad <= 8 {
								t.Errorf("align %d state %d src %q\n  kernel  nSrc=%d state=%d end=%d err=%v out=%q\n  generic nSrc=%d state=%d end=%d err=%v out=%q",
									align, st, src, n1, s1, e1, err1, d1, n2, s2, e2, err2, d2)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("kernel %s: %d inputs, %d differ", DecodeKernel(), n, bad)
}
