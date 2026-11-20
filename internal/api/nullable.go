package api

import (
	"bytes"
	"encoding/json"
)

// Nullable lets a PATCH field distinguish the three states a nullable API field
// can take. Declare the struct field as a pointer with `omitempty`:
//
//		Field *Nullable[T] `json:"field,omitempty"`
//
//	  - nil pointer            -> field omitted (keep the stored value)
//	  - NullableValue(v)       -> field serialised as v (replace the stored value)
//	  - NullableNull[T]()      -> field serialised as an explicit JSON null (remove it)
type Nullable[T any] struct {
	// Value is nil for an explicit JSON null.
	Value *T
}

// NullableValue returns a Nullable that serialises as v.
func NullableValue[T any](v T) *Nullable[T] {
	return &Nullable[T]{Value: &v}
}

// NullableNull returns a Nullable that serialises as an explicit JSON null.
func NullableNull[T any]() *Nullable[T] {
	return &Nullable[T]{}
}

// IsNull reports whether n serialises as an explicit JSON null.
func (n Nullable[T]) IsNull() bool {
	return n.Value == nil
}

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}
