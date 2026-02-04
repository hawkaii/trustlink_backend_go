package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error code and message
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a standard error response
func WriteError(w http.ResponseWriter, status int, code string, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// Unauthorized writes a 401 error
func Unauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, "unauthorized", message)
}

// BadRequest writes a 400 error
func BadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "bad_request", message)
}

// InternalServerError writes a 500 error
func InternalServerError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, "internal_server_error", message)
}

// NotFound writes a 404 error
func NotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, "not_found", message)
}

// Success writes a 200 success response
func Success(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusOK, data)
}

// Created writes a 201 created response
func Created(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusCreated, data)
}
