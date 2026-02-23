package service

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type jsonValue struct {
	any
}

// Value implements sql driver Valuer interface.
func (jv jsonValue) Value() (driver.Value, error) {
	if jv.any == nil {
		return nil, nil
	}

	var buff bytes.Buffer
	enc := json.NewEncoder(&buff)
	enc.SetEscapeHTML(false)
	err := enc.Encode(jv.any)
	if err != nil {
		return nil, err
	}

	return buff.Bytes(), err
}

// Scan implements sql driver scanner interface.
func (jv *jsonValue) Scan(value any) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("unexpected jsonb, got %T", value)
	}

	return json.Unmarshal(b, &jv.any)
}
