package procfork

import "testing"

func TestRSSCap_ScalesWithBudgetWithinBounds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		class  Class
		budget int64
		want   int64
	}{
		{"scan on 2 GiB NAS floors at 512 MiB", Scan, 2 * gib, 512 * mib},
		{"scan on 16 GiB VM caps at 1 GiB", Scan, 16 * gib, gib},
		{"interactive on 16 GiB caps at 1 GiB", Interactive, 16 * gib, gib},
		{"production on 2 GiB floors at 1.5 GiB", Production, 2 * gib, 3 * gib / 2},
		{"production on 16 GiB is budget/4", Production, 16 * gib, 4 * gib},
		{"production on 64 GiB caps at 4 GiB", Production, 64 * gib, 4 * gib},
		{"unknown budget is sized as 4 GiB", Production, 0, 3 * gib / 2},
		{"probe has no cap", Probe, 16 * gib, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RSSCap(tc.class, tc.budget); got != tc.want {
				t.Fatalf("got %d MiB, want %d MiB", got>>20, tc.want>>20)
			}
		})
	}
}

func TestThreads_CPUBindsOnTheVM_RAMBindsOnTheNAS(t *testing.T) {
	t.Parallel()
	vm := RSSCap(Production, 16*gib) // 4 GiB
	nas := RSSCap(Production, 2*gib) // 1.5 GiB
	small := RSSCap(Scan, 2*gib)     // 512 MiB
	tiny := 256 * mib
	unknown := Source{} // 4K 10-bit: 25 MB frames
	hd := Source{Width: 1920, Height: 1080}
	cases := []struct {
		name   string
		class  Class
		src    Source
		cpu    int
		rssCap int64
		want   int
	}{
		{"scan is always one thread", Scan, unknown, 7, vm, 1},
		{"production 7 vCPU 16 GiB: CPU binds at 3", Production, unknown, 7, vm, 3},
		{"production 16 cores: CPU cap 8", Production, hd, 16, vm, 8},
		{"production 4K on 1.5 GiB: CPU binds at 4", Production, unknown, 8, nas, 4},
		{"production 4K on a 512 MiB cap: RAM binds at 2", Production, unknown, 8, small, 2},
		{"production 1080p 8-bit on 1.5 GiB: CPU binds", Production, hd, 8, nas, 4},
		{"interactive never above 4", Interactive, hd, 16, vm, 4},
		{"never below one thread", Production, unknown, 8, tiny, 1},
		{"one core: one thread", Production, hd, 1, vm, 1},
		{"no cap: CPU share alone", Production, unknown, 8, 0, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Threads(tc.class, tc.src, tc.cpu, tc.rssCap); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSource_FrameBytes(t *testing.T) {
	t.Parallel()
	if got := (Source{}).frameBytes(); got != 3840*2160*3 {
		t.Fatalf("unknown source must be 4K 10-bit, got %d", got)
	}
	if got := (Source{Width: 1920, Height: 1080}).frameBytes(); got != 1920*1080*3/2 {
		t.Fatalf("1080p 8-bit, got %d", got)
	}
}
