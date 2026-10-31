package sysinfo

import (
	"math"
	"testing"
	"time"
)

func TestPercent(t *testing.T) {
	t0 := time.Unix(1000, 0)
	prev := reading{at: t0, proc: 2 * time.Second, busy: 10 * time.Second, total: 100 * time.Second, system: true}
	now := reading{at: t0.Add(2 * time.Second), proc: 3 * time.Second, busy: 13 * time.Second, total: 108 * time.Second, system: true}
	cpu := percent(prev, now, 4)
	// 1 s of CPU in 2 s on 4 cores = 12.5% of the machine.
	if math.Abs(cpu.Process-12.5) > 0.01 {
		t.Errorf("process = %v", cpu.Process)
	}
	if cpu.System == nil || math.Abs(*cpu.System-37.5) > 0.01 {
		t.Errorf("system = %v", cpu.System)
	}
	now.system = false
	if percent(prev, now, 4).System != nil {
		t.Error("system CPU without readings")
	}
}

func TestParseProcStat(t *testing.T) {
	busy, total, ok := parseProcStat("cpu  100 5 50 800 20 3 2 10 0 0\ncpu0 1 2 3\n", 100)
	// busy = user+nice+system+irq+softirq+steal = 100+5+50+3+2+10; total adds idle+iowait.
	if !ok || busy != 1700*time.Millisecond || total != 9900*time.Millisecond {
		t.Errorf("busy %v total %v ok %v", busy, total, ok)
	}
	if _, _, ok := parseProcStat("garbage", 100); ok {
		t.Error("garbage parsed")
	}
}

func TestParseNvidiaSmi(t *testing.T) {
	gpus := parseNvidiaSmi("NVIDIA GeForce RTX 3060, 12, 3\nNVIDIA T400, [N/A], 0\n")
	if len(gpus) != 2 || gpus[0].Name != "NVIDIA GeForce RTX 3060" || gpus[0].Utilization != 12 || gpus[0].Encoder != 3 {
		t.Fatalf("gpus = %+v", gpus)
	}
	if gpus[1].Utilization != 0 {
		t.Errorf("N/A = %v", gpus[1].Utilization)
	}
	if parseNvidiaSmi("") != nil {
		t.Error("empty output")
	}
}

func TestSamplerAndMemory(t *testing.T) {
	var s Sampler
	cpu := s.CPU()
	if cpu.Process < 0 || cpu.Process > 100 {
		t.Errorf("process CPU = %v", cpu.Process)
	}
	if m := Memory(); m.Bytes == 0 || m.Heap == 0 {
		t.Errorf("memory = %+v", m)
	}
}
