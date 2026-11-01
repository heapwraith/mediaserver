package intro

import (
	"math/rand/v2"
	"testing"
	"time"
)

func noise(r *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.Uint32()
	}
	return out
}

// episodes returns two fingerprints sharing `shared` points at posA and posB;
// in b the shared part is a noisy copy (a few flipped bits, a sound effect).
func episodes(shared, posA, posB int) (a, b []uint32) {
	r := rand.New(rand.NewPCG(1, 2))
	a, b = noise(r, 4800), noise(r, 4800)
	song := noise(r, shared)
	copy(a[posA:], song)
	for i, v := range song {
		switch {
		case i%40 == 7: // a door slam over the music
			v = r.Uint32()
		case i%2 == 0:
			v ^= 1<<(i%32) | 1<<((i+5)%32) | 1<<((i+11)%32)
		}
		b[posB+i] = v
	}
	return a, b
}

func near(t *testing.T, name string, got, want time.Duration) {
	t.Helper()
	if d := got - want; d < -time.Second || d > time.Second {
		t.Errorf("%s = %v, want %v ± 1 s", name, got, want)
	}
}

func TestMatch(t *testing.T) {
	a, b := episodes(250, 400, 1200) // ~31 s of opening
	inA, inB, ok := Match(a, b, 15*time.Second, 2*time.Minute)
	if !ok {
		t.Fatal("opening not found")
	}
	near(t, "a start", inA.Start, 400*Step)
	near(t, "a end", inA.End, 650*Step)
	near(t, "b start", inB.Start, 1200*Step)
	near(t, "b end", inB.End, 1450*Step)
}

func TestMatchRejects(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	if _, _, ok := Match(noise(r, 4800), noise(r, 4800), 15*time.Second, 2*time.Minute); ok {
		t.Error("unrelated audio matched")
	}
	a, b := episodes(60, 100, 900) // ~7 s: a logo, not an opening
	if _, _, ok := Match(a, b, 15*time.Second, 2*time.Minute); ok {
		t.Error("7 s accepted as an opening")
	}
	a, b = episodes(1500, 100, 900) // ~3 min: the same episode twice, not an opening
	if _, _, ok := Match(a, b, 15*time.Second, 2*time.Minute); ok {
		t.Error("3 min accepted as an opening")
	}
	silence := make([]uint32, 4800) // silence matches silence, but anchors nothing
	if _, _, ok := Match(silence, silence, 15*time.Second, 2*time.Minute); ok {
		t.Error("silence matched")
	}
}
