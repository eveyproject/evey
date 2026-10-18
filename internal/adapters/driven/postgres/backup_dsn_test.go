package postgres

import (
	"testing"

	"github.com/lib/pq"
)

// The dumper opens its own connections (table listing, schema version) and
// built their DSN with a bare Sprintf: a password with a space or a quote gave
// `missing "=" after "'word"` and every backup failed before pg_dump ran.
func TestDatabaseDumper_DSNSurvivesAPasswordWithSpacesAndQuotes(t *testing.T) {
	d := NewDatabaseDumper("db", 5432, "evey", `p@ss 'word\x`, "evey", "disable")
	if _, err := pq.NewConnector(d.dsn()); err != nil {
		t.Fatalf("dsn does not parse: %v", err)
	}
}
