package httputil

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

func BadRequest(w http.ResponseWriter, msg string) {
	Error(w, http.StatusBadRequest, "INVALID_REQUEST", msg)
}

func Unauthorized(w http.ResponseWriter) {
	Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
}

func Forbidden(w http.ResponseWriter, msg string) {
	Error(w, http.StatusForbidden, "FORBIDDEN", msg)
}

func NotFound(w http.ResponseWriter, code, msg string) {
	Error(w, http.StatusNotFound, code, msg)
}

func Conflict(w http.ResponseWriter, code, msg string) {
	Error(w, http.StatusConflict, code, msg)
}

func InternalError(w http.ResponseWriter) {
	Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func DecodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
