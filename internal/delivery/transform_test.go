package delivery

import (
	"encoding/json"
	"testing"
)

func TestTransformDataSelectsAndRenamesFields(t *testing.T) {
	result, err := TransformData([]byte(`{"id":42,"secret":"omit","nullable":null}`), []byte(`{"order_id":"id","optional":"missing","value":"nullable"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["order_id"] != float64(42) || got["value"] != nil {
		t.Fatalf("result=%s", result)
	}
}

func TestTransformRejectsExpressions(t *testing.T) {
	if err := ValidateFieldMap(map[string]string{"order_id": "user.email"}); err == nil {
		t.Fatal("expression accepted")
	}
	if _, err := TransformData([]byte(`[]`), []byte(`{"id":"id"}`)); err == nil {
		t.Fatal("array input accepted")
	}
}
