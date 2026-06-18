package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func decodeJSON(r *http.Request, v any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}
