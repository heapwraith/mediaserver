// Package sysinfo measures the server for the status page: CPU of the
// process and of the computer, memory and NVIDIA GPU use. No CGO: /proc on
// Linux, getrusage on macOS, kernel32 on Windows, nvidia-smi for the GPU.
package sysinfo

import (
	"context"
	"os/exec"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CPU use in percent of the whole computer (0–100).
type CPU struct {
	Process float64 `json:"process"`
	// System is nil where the computer's total is not read (macOS).
	System *float64 `json:"system"`
}

type reading struct {
	at          time.Time
	proc        time.Duration // CPU time used by this process
	busy, total time.Duration // CPU time of the computer
	system      bool          // busy and total are known
}

func read() reading {
	r := reading{at: time.Now(), proc: processTime()}
	r.busy, r.total, r.system = systemTimes()
	return r
}

func percent(prev, now reading, cpus int) CPU {
	var c CPU
	if wall := now.at.Sub(prev.at); wall > 0 && cpus > 0 {
		c.Process = clamp(100 * float64(now.proc-prev.proc) / (float64(wall) * float64(cpus)))
	}
	if prev.system && now.system && now.total > prev.total {
		v := clamp(100 * float64(now.busy-prev.busy) / float64(now.total-prev.total))
		c.System = &v
	}
	return c
}

func clamp(v float64) float64 { return min(100, max(0, v)) }

// Sampler measures CPU use between two calls (the status page polls).
type Sampler struct {
	mu   sync.Mutex
	last reading
}

// CPU is the use since the previous call; the first call (or one long
// after the previous) measures over a short pause.
func (s *Sampler) CPU() CPU {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := read()
	if age := now.at.Sub(s.last.at); s.last.at.IsZero() || age > time.Minute || age < 200*time.Millisecond {
		s.last = now
		time.Sleep(300 * time.Millisecond)
		now = read()
	}
	c := percent(s.last, now, runtime.NumCPU())
	s.last = now
	return c
}

// Mem is the memory of the server in bytes (Go runtime figures: the server
// has no C code, so this is close to its resident memory).
type Mem struct {
	Bytes uint64 `json:"bytes"` // obtained from the OS and not returned
	Heap  uint64 `json:"heap"`  // live heap
}

// Memory reads the runtime metrics.
func Memory() Mem {
	samples := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/memory/classes/heap/objects:bytes"},
	}
	metrics.Read(samples)
	u := func(i int) uint64 {
		if samples[i].Value.Kind() != metrics.KindUint64 {
			return 0
		}
		return samples[i].Value.Uint64()
	}
	return Mem{Bytes: u(0) - u(1), Heap: u(2)}
}

// GPU use reported by nvidia-smi, in percent.
type GPU struct {
	Name        string  `json:"name"`
	Utilization float64 `json:"utilization"`
	Encoder     float64 `json:"encoder"` // NVENC
}

var nvidiaSmi = sync.OnceValue(func() string {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return ""
	}
	return path
})

// NvidiaGPUs reads the NVIDIA GPUs; nil without nvidia-smi (Intel, AMD,
// Apple: the status page shows only the encoder in use).
func NvidiaGPUs(ctx context.Context) []GPU {
	path := nvidiaSmi()
	if path == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, //nolint:gosec // nvidia-smi found in PATH, fixed arguments
		"--query-gpu=name,utilization.gpu,utilization.encoder", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	return parseNvidiaSmi(string(out))
}

func parseNvidiaSmi(out string) []GPU {
	var gpus []GPU
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, ",")
		if len(f) != 3 {
			continue
		}
		num := func(s string) float64 {
			v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64) // [N/A] → 0
			return v
		}
		gpus = append(gpus, GPU{Name: strings.TrimSpace(f[0]), Utilization: num(f[1]), Encoder: num(f[2])})
	}
	return gpus
}

// parseProcStat reads the "cpu" line of /proc/stat (clock ticks of hz per
// second): busy and total CPU time of the computer.
func parseProcStat(stat string, hz int) (busy, total time.Duration, ok bool) {
	line, _, _ := strings.Cut(stat, "\n")
	f := strings.Fields(line)
	if len(f) < 8 || f[0] != "cpu" {
		return 0, 0, false
	}
	var v [8]int64
	for i := range v {
		n, err := strconv.ParseInt(f[i+1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		v[i] = n
	}
	// user nice system idle iowait irq softirq steal
	idle := v[3] + v[4]
	b := v[0] + v[1] + v[2] + v[5] + v[6] + v[7]
	tick := time.Second / time.Duration(hz)
	return time.Duration(b) * tick, time.Duration(b+idle) * tick, true
}
