// Package intro finds the opening and the end credits of episodes: the
// same audio appears in every episode of a season, so the longest stretch
// two episodes share near their start is the opening, and near their end,
// the credits (docs/adr/010-intro-skip.md).
package intro

import (
	"math/bits"
	"slices"
	"time"
)

// Step is the audio time of one Chromaprint point (4096-sample frames at
// 11025 Hz, a third apart).
const Step = 4096 * time.Second / 3 / 11025

const (
	maxBitErrors = 6                       // points differing in at most 6 of 32 bits are the same sound
	maxGap       = 3500 * time.Millisecond // a run survives short differences (a sound effect over the music)
	maxShifts    = 32                      // alignments tried per pair
	commonValue  = 10                      // values seen more often (silence) do not anchor alignments
	edgeWindow   = 8                       // ~1 s
)

// Span is a stretch of an episode, in its own time.
type Span struct{ Start, End time.Duration }

// Len is the length of the span.
func (s Span) Len() time.Duration { return s.End - s.Start }

// Match finds the longest stretch of audio a and b share, between minLen and
// maxLen long, and where it is in each of them.
func Match(a, b []uint32, minLen, maxLen time.Duration) (inA, inB Span, ok bool) {
	// Anchors: identical points give the candidate alignments (shift = j - i).
	index := map[uint32][]int{}
	for j, v := range b {
		index[v] = append(index[v], j)
	}
	votes := map[int]int{}
	for i, v := range a {
		if js := index[v]; len(js) <= commonValue {
			for _, j := range js {
				votes[j-i]++
			}
		}
	}
	shifts := make([]int, 0, len(votes))
	for s, n := range votes {
		if n >= 2 {
			shifts = append(shifts, s)
		}
	}
	slices.SortFunc(shifts, func(x, y int) int { // most votes first; ties in a fixed order
		if votes[x] != votes[y] {
			return votes[y] - votes[x]
		}
		return x - y
	})
	if len(shifts) > maxShifts {
		shifts = shifts[:maxShifts]
	}

	gap := int(maxGap / Step)
	best := -1
	var bestFirst, bestLast, bestShift int
	for _, s := range shifts {
		same := func(i int) bool {
			return i >= 0 && i < len(a) && i+s >= 0 && i+s < len(b) && bits.OnesCount32(a[i]^b[i+s]) <= maxBitErrors
		}
		// dense: most of the ~1 s from i (going dir) matches. The gap
		// tolerance lets stray matches before and after a run join it;
		// its edges are moved to where the shared audio really is.
		dense := func(i, dir int) bool {
			n := 0
			for k := range edgeWindow {
				if same(i + dir*k) {
					n++
				}
			}
			return n*2 > edgeWindow
		}
		first, prev := -1, -1
		try := func() {
			if first < 0 {
				return
			}
			lo, hi := first, prev
			for lo < hi && !dense(lo, 1) {
				lo++
			}
			for hi > lo && !dense(hi, -1) {
				hi--
			}
			if n := hi - lo + 1; n > best && time.Duration(n)*Step <= maxLen {
				best, bestFirst, bestLast, bestShift = n, lo, hi, s
			}
		}
		for i := max(0, -s); i < len(a) && i+s < len(b); i++ {
			if !same(i) {
				continue
			}
			if first < 0 || i-prev > gap {
				try()
				first = i
			}
			prev = i
		}
		try()
	}
	if best < 0 || time.Duration(best)*Step < minLen {
		return Span{}, Span{}, false
	}
	at := func(i int) time.Duration { return time.Duration(i) * Step }
	return Span{at(bestFirst), at(bestLast + 1)}, Span{at(bestFirst + bestShift), at(bestLast + 1 + bestShift)}, true
}
