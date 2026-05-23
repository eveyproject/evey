//go:build linux

package pluginrunner

import "testing"

func TestCapEffHasSetUIDGID(t *testing.T) {
	// CAP_SETGID=6 (0x40), CAP_SETUID=7 (0x80); both set = 0xc0.
	cases := []struct {
		name   string
		status string
		want   bool
	}{
		{"both set", "Name:\tevey\nCapEff:\t00000000000000c0\nGid:\t1000\n", true},
		{"only setuid", "CapEff:\t0000000000000080\n", false},
		{"only setgid", "CapEff:\t0000000000000040\n", false},
		{"none", "CapEff:\t0000000000000000\n", false},
		{"full root caps", "CapEff:\t000001ffffffffff\n", true},
		{"missing line", "Name:\tevey\nGid:\t1000\n", false},
		{"malformed hex", "CapEff:\tnot-hex\n", false},
	}
	for _, c := range cases {
		if got := capEffHasSetUIDGID(c.status); got != c.want {
			t.Errorf("%s: capEffHasSetUIDGID = %v, want %v", c.name, got, c.want)
		}
	}
}
