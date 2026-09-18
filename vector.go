package activeso

import (
	"encoding/json"
	"fmt"
)

// Vector32 is a dense, 32-bit floating point vector stored with Turso's vector32 type.
type Vector32 []float32

// vectorJSON serializes a vector for Turso's vector32 SQL function.
func vectorJSON(vector Vector32) (string, error) {
	// Initialize Variables
	encoded, err := json.Marshal(vector)
	if err != nil {
		return "", fmt.Errorf("activeso: encode vector32: %w", err)
	}

	return string(encoded), nil
}

// parseVector32 decodes the JSON returned by Turso's vector_extract SQL function.
func parseVector32(value any) (Vector32, error) {
	// Initialize Variables
	var raw []byte
	var vector Vector32

	// Normalize the driver value into JSON bytes.
	switch typedValue := value.(type) {
	case nil:
		return nil, nil
	case string:
		raw = []byte(typedValue)
	case []byte:
		raw = typedValue
	default:
		return nil, fmt.Errorf("activeso: decode vector32 from %T", value)
	}

	if err := json.Unmarshal(raw, &vector); err != nil {
		return nil, fmt.Errorf("activeso: decode vector32: %w", err)
	}

	return vector, nil
}
