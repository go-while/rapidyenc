package rapidyenc

import "math/rand/v2"

var zzAllStates = []State{StateCRLF, StateEQ, StateCR, StateNone, StateCRLFDT, StateCRLFDTCR, StateCRLFEQ}

// zzBoundaryInputs calls f for every boundary input of TestZZBoundaries and
// TestZZAnalyze (all lengths): every string of up to 4 of the bytes "=\r\n.ya"
// at offsets 0..140 of plain data, with tails of 0, 3, 24, 47, 70 and 140
// bytes, and all start states when the pattern is near the start.
func zzBoundaryInputs(f func(src, pat []byte, off, tail int, states []State)) {
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
	for _, pat := range pats {
		for off := 0; off <= 140; off++ {
			for _, tail := range []int{0, 3, 24, 47, 70, 140} {
				src := append(append(append([]byte(nil), plain[:off]...), pat...), plain[off:off+tail]...)
				states := []State{StateNone, StateCRLF}
				if off < 4 {
					states = zzAllStates
				}
				f(src, pat, off, tail, states)
			}
		}
	}
}

// zzRandomInput returns a pseudo-random yEnc-ish input of 1..maxLen bytes:
// plain bytes mixed with "=\r\n.y" at a density chosen per input, and now
// and then a whole sequence such as "\r\n.", "\r\n=y", "\r\n.\r\n" or "==".
func zzRandomInput(r *rand.Rand, maxLen int) []byte {
	n := 1 + r.IntN(maxLen)
	dens := []float64{0.005, 0.03, 0.1, 0.3, 0.7}[r.IntN(5)]
	seqs := []string{"\r\n.", "\r\n=y", "\r\n.\r\n", "==", "\r\n.=y", "=\r\n", "\r\n..", "\r\n", "=\r", "\r\n.\r"}
	b := make([]byte, 0, n+8)
	for len(b) < n {
		switch x := r.Float64(); {
		case x < dens*0.15:
			b = append(b, seqs[r.IntN(len(seqs))]...)
		case x < dens:
			b = append(b, "=\r\n.y"[r.IntN(5)])
		default:
			c := byte(r.IntN(256))
			for c == '=' || c == '\r' || c == '\n' || c == '.' {
				c = byte(r.IntN(256))
			}
			b = append(b, c)
		}
	}
	return b[:n]
}
