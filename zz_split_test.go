package rapidyenc

import (
	"fmt"
	"io"
	"testing"
)

type zzChunks struct{ parts [][]byte }

func (r *zzChunks) Read(p []byte) (int, error) {
	for len(r.parts) > 0 && len(r.parts[0]) == 0 {
		r.parts = r.parts[1:]
	}
	if len(r.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.parts[0])
	r.parts[0] = r.parts[0][n:]
	return n, nil
}

func zzNext(data []byte, k int) (res string) {
	defer func() {
		if e := recover(); e != nil {
			res = fmt.Sprintf("PANIC: %v", e)
		}
	}()
	r := &zzChunks{parts: [][]byte{append([]byte(nil), data[:k]...), append([]byte(nil), data[k:]...)}}
	resp, err := NewDecoder(r, WithStatusLineAlreadyRead()).Next()
	if resp == nil {
		return fmt.Sprintf("nil response, err %v", err)
	}
	return fmt.Sprintf("data %q err %v", resp.Data, err)
}

// TestZZSplit decodes an article delivered in two reads, split at every
// position, and compares with one read.
func TestZZSplit(t *testing.T) {
	for _, tc := range []struct{ name, art string }{
		{"with =yend", "=ybegin line=128 size=3 name=x\r\n\x8b\x8c\x8d\r\n=yend size=3\r\n.\r\n"},
		{"no =yend", "=ybegin line=128 size=3 name=x\r\n\x8b\x8c\x8d\r\n.\r\n"},
	} {
		data := []byte(tc.art)
		whole := zzNext(data, len(data))
		t.Logf("%s, one read: %s", tc.name, whole)
		for k := 1; k < len(data); k++ {
			if got := zzNext(data, k); got != whole {
				t.Errorf("%s, split after %d bytes (%q | %q): %s", tc.name, k, data[:k], data[k:], got)
			}
		}
	}
}
