package delivery

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// ValidateFieldMap permits only top-level field selection and renaming.
// No expressions, external lookups, or arbitrary code can run in a worker.
func ValidateFieldMap(m map[string]string) error {
	if len(m) > 32 {
		return errors.New("field map has too many entries")
	}
	for destination, source := range m {
		if !fieldName.MatchString(destination) || !fieldName.MatchString(source) {
			return errors.New("field map keys must be simple field names")
		}
	}
	return nil
}

// TransformData returns the original payload when the map is empty. Missing
// source fields are omitted, preserving explicit JSON null values.
func TransformData(payload, mapping []byte) ([]byte, error) {
	if len(mapping) == 0 || bytes.Equal(bytes.TrimSpace(mapping), []byte(`{}`)) {
		return payload, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		return nil, errors.New("mapped event data must be a JSON object")
	}
	var m map[string]string
	if err := json.Unmarshal(mapping, &m); err != nil {
		return nil, err
	}
	if err := ValidateFieldMap(m); err != nil {
		return nil, err
	}
	result := make(map[string]json.RawMessage, len(m))
	for destination, source := range m {
		if value, ok := fields[source]; ok {
			result[destination] = value
		}
	}
	return json.Marshal(result)
}
