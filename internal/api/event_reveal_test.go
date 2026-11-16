package api

import (
	"reflect"
	"testing"
)

func TestRedactEventValue(t *testing.T) {
	input := map[string]any{
		"order_id": "visible",
		"comment":  "sent to person@example.test",
		"password": "hidden",
		"nested": []any{map[string]any{
			"api_key": "hidden",
			"email":   "person@example.test",
			"note":    "Bearer abc",
			"count":   float64(2),
		}},
	}
	want := map[string]any{
		"order_id": "visible",
		"comment":  "sent to [REDACTED_EMAIL]",
		"password": "[REDACTED]",
		"nested": []any{map[string]any{
			"api_key": "[REDACTED]",
			"email":   "[REDACTED]",
			"note":    "[REDACTED]",
			"count":   float64(2),
		}},
	}
	if got := redactEventValue(input, 0); !reflect.DeepEqual(got, want) {
		t.Fatalf("redacted value=%#v", got)
	}
}
