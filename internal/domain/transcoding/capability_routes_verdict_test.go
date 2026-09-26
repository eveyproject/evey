package transcoding

import (
	"encoding/json"
	"testing"
)

// A backend that proved NO route writes the empty verdict, and that verdict
// must survive the row's persistence: Routes used to be omitted from the JSON
// when empty, came back nil, and nil read as "every route trusted" - a
// backend with no working route was offered all of them (measured 2026-09-26).
func TestRoutesVerdict_EmptyListSurvivesPersistence(t *testing.T) {
	row := EncodeCapability{
		ProbeVersion: CapabilityProbeVersion,
		Backends:     []HWBackendCap{{Backend: "vaapi", Codecs: []string{"h264"}, Routes: []string{}}},
	}
	if row.Backends[0].Proves(HWRouteSDR) {
		t.Fatal("an empty verdict must not prove a route before persistence")
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var back EncodeCapability
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Backends[0].Routes == nil {
		t.Fatalf("the empty verdict was dropped from the JSON: %s", b)
	}
	if back.Snapshot().Backends[0].Proves(HWRouteSDR) {
		t.Fatal("the reloaded row proves a route the probe never proved")
	}
}

// Rows persisted while the empty verdict was still dropped come back with a
// nil Routes at a route-probed ProbeVersion: publishing them reads nil as "no
// route proven". A row from before the route probes keeps nil, and its trust.
func TestRoutesVerdict_NilOnAProbedRowIsNoProof(t *testing.T) {
	probed := EncodeCapability{
		ProbeVersion: decodeClassProbeVersion,
		Backends:     []HWBackendCap{{Backend: "qsv", Routes: nil}, {Backend: "vaapi", Routes: []string{HWRouteSDR}}},
	}
	snap := probed.Snapshot()
	if snap.Backends[0].Routes == nil || snap.Backends[0].Proves(HWRouteSDR) {
		t.Fatalf("a probed row's nil verdict must read as no proof, got routes=%v", snap.Backends[0].Routes)
	}
	if !snap.Backends[1].Proves(HWRouteSDR) {
		t.Fatal("a proven route must stay proven")
	}
	if probed.Backends[0].Routes != nil {
		t.Fatal("the persisted row is never edited in place")
	}

	preProbe := EncodeCapability{ProbeVersion: decodeClassProbeVersion - 1, Backends: []HWBackendCap{{Backend: "qsv", Routes: nil}}}
	if !preProbe.Snapshot().Backends[0].Proves(HWRouteSDR) {
		t.Fatal("a row benched before the route probes keeps its trust until it is re-benched")
	}
}
