package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxBodyBytes = 10 << 20

type APIError struct {
	Status int    `json:"-"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail"`
}

func (e *APIError) Error() string {
	return e.Detail
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, detail, code string) {
	writeJSON(w, status, APIError{Status: status, Code: code, Detail: detail})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
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
