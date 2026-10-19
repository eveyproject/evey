package loadtest

import (
	"maps"
	"math/rand/v2"
	"slices"
	"sort"
)

// librarySampleBands is how finely LibrarySample cuts the library by bitrate.
// Deciles, not quartiles: the heaviest tenth (4K remuxes at 40 to 80 Mb/s)
// decides what an audience pulls, and a slice of 150 titles per kind left its
// top quartile floor anywhere between 6.7 and 9.7 Mb/s from one seed to the
// next (prod logs, 2026-10-06 to 2026-10-19).
const librarySampleBands = 10

// LibrarySample picks n titles out of the whole library so that every kind
// of title and every bitrate decile of the library holds its share of the
// sample, titles chosen at random within each by seed. A contiguous slice of
// ids per kind, half films and half episodes, followed neither: a library of
// ten episodes per film was drawn as many films as episodes, and each seed
// drew its own mix of heavy files, so two runs on the same server measured
// two different audiences. The same seed over the same library picks the same
// titles; another seed picks other titles in the same proportions.
func LibrarySample(library []MediaCandidate, n int, seed uint64) []MediaCandidate {
	if n <= 0 {
		return nil
	}
	if n >= len(library) {
		return library
	}
	byKind := map[string][]MediaCandidate{}
	for _, c := range library {
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x5851f42d4c957f2d)) //nolint:gosec // a replayable sample of the library, not a secret
	out := make([]MediaCandidate, 0, n)
	perKind := apportion(n, byKind, len(library))
	for _, kind := range slices.Sorted(maps.Keys(byKind)) {
		// Each kind is cut by its OWN deciles: the library's would be the
		// episodes' (ten per film), and every film above 10 Mb/s would share
		// one band, 4K remuxes and light 1080p alike.
		titles := byKind[kind]
		bands, _ := bitrateQuantiles(titles, librarySampleBands)
		byBand := map[string][]MediaCandidate{}
		for i, c := range titles {
			byBand[bands[i]] = append(byBand[bands[i]], c)
		}
		perBand := apportion(perKind[kind], byBand, len(titles))
		for _, b := range slices.Sorted(maps.Keys(byBand)) {
			list := slices.Clone(byBand[b])
			rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
			out = append(out, list[:perBand[b]]...)
		}
	}
	return out
}

// apportion shares n seats among the groups in proportion to their sizes out
// of total: each group gets the whole part of its share, then the seats left
// go to the largest remainders, ties to the first group in name order. No
// group gets more seats than it has members while n <= total.
func apportion(n int, groups map[string][]MediaCandidate, total int) map[string]int {
	keys := slices.Sorted(maps.Keys(groups))
	seats := make(map[string]int, len(keys))
	rem := make(map[string]float64, len(keys))
	left := n
	for _, k := range keys {
		exact := float64(n) * float64(len(groups[k])) / float64(max(total, 1))
		seats[k] = int(exact)
		rem[k] = exact - float64(seats[k])
		left -= seats[k]
	}
	byRem := slices.Clone(keys)
	sort.SliceStable(byRem, func(i, j int) bool { return rem[byRem[i]] > rem[byRem[j]] })
	for i := 0; left > 0 && len(byRem) > 0; i++ {
		seats[byRem[i%len(byRem)]]++
		left--
	}
	return seats
}
