package grpcapi

import (
	"fmt"
	"strconv"
)

func coerceConfigValues(values map[string]string, schema map[string]any) map[string]any {
	cfg := make(map[string]any, len(values))
	for k, v := range values {
		fs, ok := fieldSchema(schema, k)
		if !ok {
			cfg[k] = v
			continue
		}
		switch fs["type"] {
		case "boolean":
			cfg[k] = v == "true"
		case "integer":
			if n, err := strconv.Atoi(v); err == nil {
				cfg[k] = n
			} else {
				cfg[k] = v
			}
		case "number":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				cfg[k] = f
			} else {
				cfg[k] = v
			}
		default:
			cfg[k] = v
		}
	}
	return cfg
}

func validateConfigValues(values map[string]string, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	for k, raw := range schema {
		fs, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		v, exists := values[k]
		if req, _ := fs["required"].(bool); req && (!exists || v == "") {
			return fmt.Errorf("%s is required", k)
		}
		if !exists || v == "" {
			continue
		}
		typ, _ := fs["type"].(string)
		if typ == "integer" || typ == "number" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("%s must be a number", k)
			}
			if typ == "integer" && float64(int64(n)) != n {
				return fmt.Errorf("%s must be an integer", k)
			}
			if minVal, ok := fs["min"]; ok {
				if m, _ := toFloat64(minVal); n < m {
					return fmt.Errorf("%s must be >= %v", k, minVal)
				}
			}
			if maxVal, ok := fs["max"]; ok {
				if m, _ := toFloat64(maxVal); n > m {
					return fmt.Errorf("%s must be <= %v", k, maxVal)
				}
			}
		}
	}
	return nil
}

func fieldSchema(schema map[string]any, key string) (map[string]any, bool) {
	raw, ok := schema[key]
	if !ok {
		return nil, false
	}
	fs, ok := raw.(map[string]any)
	return fs, ok
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}
