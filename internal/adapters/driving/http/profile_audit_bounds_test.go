package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The field names of a body are the caller's to choose and reach the audit
// log even when the request is refused: a row keeps a bounded number of
// them, each of a bounded size, and never half a character.
func TestJSONFieldNames_Bounded(t *testing.T) {
	body := map[string]int{}
	for i := range auditMaxFieldNames + 10 {
		body[fmt.Sprintf("k%03d", i)] = i
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	names := jsonFieldNames(raw)
	require.Len(t, names, auditMaxFieldNames)
	assert.Equal(t, "k000", names[0], "the first names in order are the ones kept")

	long := strings.Repeat("a", auditMaxFieldNameSize-1) + strings.Repeat("é", 40)
	raw, err = json.Marshal(map[string]int{long: 1, "pin": 2})
	require.NoError(t, err)
	names = jsonFieldNames(raw)
	require.Len(t, names, 2)
	assert.LessOrEqual(t, len(names[0]), auditMaxFieldNameSize)
	assert.True(t, utf8.ValidString(names[0]))
	assert.Equal(t, "pin", names[1], "a short name is kept as it is")

	assert.Nil(t, jsonFieldNames(nil))
	assert.Nil(t, jsonFieldNames([]byte(`[1,2]`)))
}
