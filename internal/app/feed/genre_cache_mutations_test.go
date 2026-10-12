package feed_test

import (
	"context"
	"testing"
	"time"

	appfeed "github.com/eveyproject/evey/internal/app/feed"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// itoa is private; we probe it via cacheKey collisions: distinct minCounts for the
// same profile must produce distinct cache entries. Any itoa mutation collapsing the
// outputs causes the second Get call to return the first's cached genres.

// L58 negation: if n == 0 -> if n != 0.
// Expected: itoa(0)="0", itoa(5)="5". Mutated: itoa(0)="" via digit loop (n>0 false => empty),
// itoa(5)="0" via early return. Collision: minCount=0 and minCount=5 keys would be "p:" vs "p:0"
// (different) normally; mutated gives "p:" and "p:0" too. Better: minCount=0 vs minCount=5 yield
// "p:0" vs "p:5" normally; mutated yields "p:" vs "p:0". Both pairs distinct. Need direct collision:
// minCount=5 vs minCount=50. Normal: "p:5","p:50". Mutated L58: "p:0","p:0" COLLISION.
func TestCachedAnalyzer_Itoa_ZeroCheck(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"a"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)
	ctx := context.Background()

	// Prime cache for minCount=5.
	g1, err := cached.GetFavoriteGenres(ctx, "p", 5)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, g1)

	// Change underlying result; fetch with minCount=50. Distinct key required.
	fake.Genres = []string{"b"}
	g2, err := cached.GetFavoriteGenres(ctx, "p", 50)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, g2, "minCount=5 and minCount=50 must be distinct cache keys")
}

// L63 negation: neg := n < 0 -> n >= 0. Mutated: itoa(5) treats 5 as negative (neg=true, n=-5
// overflow? actually n = -n = -5; then for n>0 false, loop skipped; result buf with '-' only = "-").
// itoa(-5): mutated neg=false, n stays -5, loop n>0 false, returns "" (empty).
// Normal: itoa(-5)="-5".
// Collision test: minCount=5 vs minCount=-5. Normal keys "p:5","p:-5" distinct. Mutated: "p:-","p:"
// distinct too. Hmm.
// Better: use key collision via itoa(5)="-" and itoa(-5)="" - those differ. Fine.
// Actual kill: itoa(-5) vs itoa(-50) - normal "-5","-50" (distinct); mutated: both "" (collision).
func TestCachedAnalyzer_Itoa_NegativeSign(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"x"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)
	ctx := context.Background()

	g1, err := cached.GetFavoriteGenres(ctx, "p", -5)
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, g1)

	fake.Genres = []string{"y"}
	g2, err := cached.GetFavoriteGenres(ctx, "p", -50)
	require.NoError(t, err)
	assert.Equal(t, []string{"y"}, g2, "minCount=-5 and -50 must be distinct keys (sign logic)")

	// And positive vs negative of same magnitude must differ.
	fake.Genres = []string{"z"}
	g3, err := cached.GetFavoriteGenres(ctx, "p", 5)
	require.NoError(t, err)
	// key "p:5" vs earlier "p:-5"; must be fresh fetch.
	assert.Equal(t, []string{"z"}, g3, "minCount=5 vs -5 must be distinct keys")
}

// L67 negation: for n > 0 -> for n <= 0. Mutated: positive n never loops => itoa(5)="" (or "-" for neg).
// Collision: minCount=5 vs minCount=7 both produce "" => keys "p:" and "p:" collide.
func TestCachedAnalyzer_Itoa_LoopRuns(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"a"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)
	ctx := context.Background()

	g1, err := cached.GetFavoriteGenres(ctx, "p", 5)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, g1)

	fake.Genres = []string{"b"}
	g2, err := cached.GetFavoriteGenres(ctx, "p", 7)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, g2, "minCount=5 vs 7 distinct keys (loop must execute)")
}

// L69 arithmetic: buf[pos] = byte('0' + n%10). Mutations:
//
//	'0' + n%10 -> '0' - n%10, '0' * n%10, '0' / n%10, '0' % n%10.
//
// Also n%10 -> n/10, n*10, n+10, n-10.
// Collision: minCount=12 vs minCount=21. Normal: keys "p:12","p:21" distinct.
// '0' - n%10 mutation: 12 -> '0'-2,'0'-1 = chars, but deterministic; 21 -> '0'-1,'0'-2. Still distinct.
// Try minCount=12 vs minCount=13: normal "p:12","p:13"; n%10 mutated to n/10 -> both 1,1 => "p:11","p:11" COLLISION.
// minCount=10 vs minCount=20: n%10 mutated to n*10: 10%10=0,10*10=100 (byte overflow). Varies.
// Cover broadly: use pairs 12/13 (covers n%10 mutations), and 12/21 (covers '0'+ mutations via digit order).
func TestCachedAnalyzer_Itoa_DigitComputation(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"a"}
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute)
	ctx := context.Background()

	// 12 vs 13 - differ only in last digit (n%10 mutations).
	_, err := cached.GetFavoriteGenres(ctx, "p", 12)
	require.NoError(t, err)

	fake.Genres = []string{"b"}
	g2, err := cached.GetFavoriteGenres(ctx, "p", 13)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, g2, "minCount=12 vs 13 distinct (n%10 last-digit)")

	// 12 vs 21 - same digits swapped (byte math '0'+d order matters).
	fake.Genres = []string{"c"}
	g3, err := cached.GetFavoriteGenres(ctx, "p", 21)
	require.NoError(t, err)
	assert.Equal(t, []string{"c"}, g3, "minCount=12 vs 21 distinct (digit ordering)")

	// Extra: 0-padding via n/10 division - 5 vs 15 distinct.
	fake.Genres = []string{"d"}
	_, err = cached.GetFavoriteGenres(ctx, "q", 5)
	require.NoError(t, err)
	fake.Genres = []string{"e"}
	g5, err := cached.GetFavoriteGenres(ctx, "q", 15)
	require.NoError(t, err)
	assert.Equal(t, []string{"e"}, g5, "minCount=5 vs 15 distinct (n/10 loop termination)")
}

// L83 boundary: time.Since(entry.fetchedAt) < c.ttl -> <=. Logically equivalent at nanosecond
// precision. We exercise both states: fresh (cached) and expired (refetched).
func TestCachedAnalyzer_TTL_FreshVsExpired(t *testing.T) {
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"a"}
	cached := appfeed.NewCachedAnalyzer(fake, 20*time.Millisecond)
	ctx := context.Background()

	_, err := cached.GetFavoriteGenres(ctx, "p", 3)
	require.NoError(t, err)

	// Immediately: still fresh.
	fake.Genres = []string{"b"}
	g, err := cached.GetFavoriteGenres(ctx, "p", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, g)

	// After TTL: refetch.
	time.Sleep(40 * time.Millisecond)
	g2, err := cached.GetFavoriteGenres(ctx, "p", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, g2)
}
