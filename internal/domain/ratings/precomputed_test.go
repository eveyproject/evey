package ratings

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// precomputedRatings is every table key, the same keys as TMDB also spells
// them (agency prefix, lower case, padding), and labels no table knows.
func precomputedRatings() []string {
	out := []string{"", "  ", "bogus", "NR", "ZZ-bogus"}
	for c, table := range byCountry {
		for r := range table {
			out = append(out, r, " "+r+" ")
			for p, pc := range agencyPrefixes {
				if pc == c {
					out = append(out, p+" "+r, p+"-"+r)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func precomputedRegions() []string {
	out := []string{"BE", "ZZ"} // no table of their own: the US fallback alone
	for c := range byCountry {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// The stored half and the region half recombine into WorstCaseMinAge for any
// region, official rating and certification set, so a database applying them
// hides exactly what the Go filter hides.
func TestPrecomputedMatchesWorstCaseMinAge(t *testing.T) {
	ratingsList := precomputedRatings()
	regions := precomputedRegions()
	rng := rand.New(rand.NewPCG(1, 2))
	countries := append(precomputedRegions(), "", "xx")
	randomCerts := func() map[string]string {
		n := rng.IntN(4)
		if n == 0 {
			return nil
		}
		m := make(map[string]string, n)
		for range n {
			m[countries[rng.IntN(len(countries))]] = ratingsList[rng.IntN(len(ratingsList))]
		}
		return m
	}
	for _, region := range regions {
		for _, official := range ratingsList {
			for range 8 {
				certs := randomCerts()
				want := WorstCaseMinAge(region, official, certs)
				got := max(CertificationsMinAge(certs), minAgeForKey(region, RatingKey(official)))
				if got != want {
					t.Fatalf("region %q official %q certs %v: precomputed %d, WorstCaseMinAge %d", region, official, certs, got, want)
				}
			}
		}
	}
}

// Every key a region can resolve lands in exactly one list, on the right side
// of the cap, and a key absent from both has no known age there.
func TestRatingKeysForRegionPartitionsByAge(t *testing.T) {
	for _, region := range precomputedRegions() {
		for _, maxAge := range []int{0, 6, 10, 12, 13, 16, 18} {
			allowed, blocked := RatingKeysForRegion(region, maxAge)
			for _, official := range precomputedRatings() {
				key := RatingKey(official)
				age := minAgeForKey(region, key)
				inAllowed, inBlocked := slices.Contains(allowed, key), slices.Contains(blocked, key)
				switch {
				case age < 0 && (inAllowed || inBlocked):
					t.Fatalf("%s/%d: %q has no age but is listed", region, maxAge, key)
				case age >= 0 && age <= maxAge && !inAllowed:
					t.Fatalf("%s/%d: %q (age %d) missing from allowed", region, maxAge, key, age)
				case age > maxAge && !inBlocked:
					t.Fatalf("%s/%d: %q (age %d) missing from blocked", region, maxAge, key, age)
				}
			}
		}
	}
}

func TestTablesRevisionFollowsTheTables(t *testing.T) {
	before := tablesRevision()
	if before != TablesRevision {
		t.Fatalf("revision not stable: %d then %d", TablesRevision, before)
	}
	byCountry["FR"]["TP"]++
	defer func() { byCountry["FR"]["TP"]-- }()
	if tablesRevision() == before {
		t.Fatal("editing an age left the revision unchanged")
	}
}
