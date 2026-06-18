package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func decodeJSON(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}
