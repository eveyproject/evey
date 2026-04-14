package postgres_test

import (
	"os"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
)

func TestMain(m *testing.M) {
	cleanup := testhelpers.StartShared()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
