package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxBodyBytes = 10 << 20

type APIError struct {
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, detail, code string) {
	writeJSON(w, status, APIError{Code: code, Detail: detail})
}

func writePage(w http.ResponseWriter, items any, hasNext bool, skip, limit int) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"has_next": hasNext,
		"skip":     skip,
		"limit":    limit,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("unexpected trailing data")
	}
	return nil
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func writeRequestError(w http.ResponseWriter, err error) {
	if isBodyTooLarge(err) {
		writeError(w, http.StatusRequestEntityTooLarge, "Request body too large", "BODY_TOO_LARGE")
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "Invalid request body", "VALIDATION_ERROR")
}
