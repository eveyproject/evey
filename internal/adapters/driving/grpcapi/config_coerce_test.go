package grpcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoerceConfigValues(t *testing.T) {
	schema := map[string]any{
		"enabled": map[string]any{"type": "boolean"},
		"count":   map[string]any{"type": "integer"},
		"rate":    map[string]any{"type": "number"},
		"name":    map[string]any{"type": "string"},
	}

	values := map[string]string{
		"enabled": "true",
		"count":   "42",
		"rate":    "3.14",
		"name":    "test",
		"unknown": "val",
	}

	cfg := coerceConfigValues(values, schema)
	assert.Equal(t, true, cfg["enabled"])
	assert.Equal(t, 42, cfg["count"])
	assert.InDelta(t, 3.14, cfg["rate"], 0.001)
	assert.Equal(t, "test", cfg["name"])
	assert.Equal(t, "val", cfg["unknown"])
}

func TestCoerceConfigValues_FalseBool(t *testing.T) {
	schema := map[string]any{"debug": map[string]any{"type": "boolean"}}
	cfg := coerceConfigValues(map[string]string{"debug": "false"}, schema)
	assert.Equal(t, false, cfg["debug"])
}

func TestCoerceConfigValues_InvalidNumber(t *testing.T) {
	schema := map[string]any{"port": map[string]any{"type": "integer"}}
	cfg := coerceConfigValues(map[string]string{"port": "abc"}, schema)
	assert.Equal(t, "abc", cfg["port"])
}

func TestCoerceConfigValues_NilSchema(t *testing.T) {
	cfg := coerceConfigValues(map[string]string{"k": "v"}, nil)
	assert.Equal(t, "v", cfg["k"])
}

func TestValidateConfigValues_Required(t *testing.T) {
	schema := map[string]any{"api_key": map[string]any{"type": "string", "required": true}}

	err := validateConfigValues(map[string]string{"api_key": ""}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")

	err = validateConfigValues(map[string]string{"api_key": "abc"}, schema)
	assert.NoError(t, err)
}

func TestValidateConfigValues_RequiredMissing(t *testing.T) {
	schema := map[string]any{"api_key": map[string]any{"type": "string", "required": true}}
	err := validateConfigValues(map[string]string{}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestValidateConfigValues_MinMax(t *testing.T) {
	schema := map[string]any{
		"count": map[string]any{"type": "integer", "min": float64(1), "max": float64(100)},
	}

	err := validateConfigValues(map[string]string{"count": "0"}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ">=")

	err = validateConfigValues(map[string]string{"count": "101"}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "<=")

	err = validateConfigValues(map[string]string{"count": "50"}, schema)
	assert.NoError(t, err)
}

func TestValidateConfigValues_MustBeInteger(t *testing.T) {
	schema := map[string]any{"n": map[string]any{"type": "integer"}}
	err := validateConfigValues(map[string]string{"n": "3.5"}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integer")
}

func TestValidateConfigValues_MustBeNumber(t *testing.T) {
	schema := map[string]any{"rate": map[string]any{"type": "number"}}
	err := validateConfigValues(map[string]string{"rate": "abc"}, schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "number")
}

func TestValidateConfigValues_EmptySchema(t *testing.T) {
	err := validateConfigValues(map[string]string{"any": "thing"}, nil)
	assert.NoError(t, err)
}

func TestStringFromInterface_Types(t *testing.T) {
	assert.Equal(t, "", stringFromInterface(nil))
	assert.Equal(t, "hello", stringFromInterface("hello"))
	assert.Equal(t, "true", stringFromInterface(true))
	assert.Equal(t, "false", stringFromInterface(false))
	assert.Equal(t, "42", stringFromInterface(float64(42)))
	assert.Equal(t, "3.14", stringFromInterface(3.14))
	assert.Equal(t, "7", stringFromInterface(7))
}
