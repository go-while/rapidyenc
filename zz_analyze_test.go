package rapidyenc

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"testing"
	"unsafe"

	yencsimd "github.com/go-while/go-yenc-simd"
)

type zzRes struct {
	n     int
	out   []byte
	state int
	end   int
}

func zzOurs(src []byte, st State) zzRes {
	dst := make([]byte, len(src)+64)
	s := yencsimd.State(st) // same numbering as rapidyenc's RapidYencDecoderState
	nd, ns, end, _ := yencsimd.DecodeIncremental(dst, src, &s)
	return zzRes{ns, dst[:nd], int(s), int(end)}
}

func zzTheirs(f func(dst, src []byte, state State) (int, []byte, State, End, error), src []byte, st State) zzRes {
	return zzTheirsAt(f, src, st, -1)
}

// zzTheirsAt decodes a copy of src placed at the given address alignment
// (mod 64), or wherever the allocator puts it if align < 0.
func zzTheirsAt(f func(dst, src []byte, state State) (int, []byte, State, End, error), src []byte, st State, align int) zzRes {
	buf := make([]byte, len(src)+128)
	shift := 0
	if align >= 0 {
		base := int(uintptr(unsafe.Pointer(&buf[0])) & 63)
		shift = (align - base + 64) & 63
	}
	s := buf[shift : shift+len(src)]
	copy(s, src)
	dst := make([]byte, len(src)+64)
	n, out, ps, end, _ := f(dst, s, st)
	return zzRes{n, append([]byte(nil), out...), int(ps), int(end)}
}

func (a zzRes) diff(b zzRes) string {
	d := ""
	if a.n != b.n {
		d += "nSrc "
	}
	if !bytes.Equal(a.out, b.out) {
		d += "out "
	}
	if a.state != b.state {
		d += "state "
	}
	if a.end != b.end {
		d += "end "
	}
	return d
}

// zzClassify compares the kernel (decodeIncremental), decodeGeneric and
// go-yenc-simd's state machine on one input. It returns "" if all three
// agree, otherwise a group name, and whether the group is the known
// decodeGeneric bug: input ends in "\r\n", kernel == generic, and both
// return nSrc = len-2 where ours returns len, everything else equal.
func zzClassify(src []byte, k, g, o zzRes) (who string, known bool) {
	kg, ko, gO := k.diff(g), k.diff(o), g.diff(o)
	if kg == "" && ko == "" {
		return "", false
	}
	switch {
	case ko == "" && gO != "":
		who = "generic differs from kernel and ours: " + gO
	case gO == "" && ko != "":
		who = "kernel differs from generic and ours: " + ko
	case kg == "" && ko != "":
		who = "ours differs from kernel and generic: " + ko
	default:
		who = "all three differ: kernel/gen " + kg + "| kernel/ours " + ko + "| gen/ours " + gO
	}
	known = kg == "" && ko == "nSrc " && bytes.HasSuffix(src, []byte("\r\n")) &&
		o.n == len(src) && k.n == len(src)-2
	if known {
		who = "KNOWN decodeGeneric nSrc bug (kernel == generic, nSrc = len-2, ours len): " + ko
	}
	return who, known
}

type zzGroups struct {
	n        int
	groups   map[string]int
	examples map[string][]string
	unknown  int
}

func (z *zzGroups) add(src []byte, desc string, k, g, o zzRes) {
	z.n++
	who, known := zzClassify(src, k, g, o)
	if who == "" {
		return
	}
	if !known {
		z.unknown++
	}
	key := fmt.Sprintf("%s [len<=128: %v]", who, len(src) <= 128)
	if z.groups == nil {
		z.groups, z.examples = map[string]int{}, map[string][]string{}
	}
	z.groups[key]++
	if len(z.examples[key]) < 6 {
		tl := src
		if len(tl) > 12 {
			tl = tl[len(tl)-12:]
		}
		z.examples[key] = append(z.examples[key], fmt.Sprintf("len %d %s ...%q | kernel n=%d st=%d end=%d out=%d B | gen n=%d st=%d end=%d out=%d B | ours n=%d st=%d end=%d out=%d B",
			len(src), desc, tl, k.n, k.state, k.end, len(k.out), g.n, g.state, g.end, len(g.out), o.n, o.state, o.end, len(o.out)))
		if len(src) <= 300 && len(z.examples[key]) <= 2 {
			z.examples[key] = append(z.examples[key], fmt.Sprintf("    src %q\n    kernel out %q\n    gen    out %q\n    ours   out %q", src, k.out, g.out, o.out))
		}
	}
}

func (z *zzGroups) report(t *testing.T) {
	keys := make([]string, 0, len(z.groups))
	for k := range z.groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("kernel %s: %d inputs, %d in groups other than the known one", DecodeKernel(), z.n, z.unknown)
	for _, k := range keys {
		t.Logf("%7d  %s", z.groups[k], k)
		for _, e := range z.examples[k] {
			t.Logf("         %s", e)
		}
	}
	if z.unknown > 0 {
		t.Errorf("%d inputs disagree outside the known decodeGeneric nSrc group", z.unknown)
	}
}

// TestZZAnalyze runs every boundary input (all lengths) through
// decodeIncremental (the kernel: NEON on arm64 with !cgo && simd),
// decodeGeneric and go-yenc-simd's state machine, and groups the
// disagreements. It fails on any disagreement outside the known
// decodeGeneric nSrc group.
func TestZZAnalyze(t *testing.T) {
	if err := yencsimd.SetKernel("scalar"); err != nil {
		t.Fatal(err)
	}
	var z zzGroups
	zzBoundaryInputs(func(src, pat []byte, off, tail int, states []State) {
		for _, st := range states {
			desc := fmt.Sprintf("state %d pat %q off %d tail %d", st, pat, off, tail)
			z.add(src, desc, zzTheirs(decodeIncremental, src, st), zzTheirs(decodeGeneric, src, st), zzOurs(src, st))
		}
	})
	z.report(t)
}

func zzEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// TestZZRandom is TestZZAnalyze on pseudo-random inputs of up to 2048 bytes,
// at random alignments and start states. ZZ_RANDOM_N sets the count and
// ZZ_SEED the seed.
func TestZZRandom(t *testing.T) {
	if err := yencsimd.SetKernel("scalar"); err != nil {
		t.Fatal(err)
	}
	n, seed := zzEnvInt("ZZ_RANDOM_N", 200000), zzEnvInt("ZZ_SEED", 1)
	t.Logf("seed %d, %d inputs", seed, n)
	r := rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))
	var z zzGroups
	for range n {
		src := zzRandomInput(r, 2048)
		st := zzAllStates[r.IntN(len(zzAllStates))]
		align := r.IntN(64)
		desc := fmt.Sprintf("state %d align %d", st, align)
		z.add(src, desc, zzTheirsAt(decodeIncremental, src, st, align), zzTheirsAt(decodeGeneric, src, st, align), zzOurs(src, st))
	}
	z.report(t)
}
