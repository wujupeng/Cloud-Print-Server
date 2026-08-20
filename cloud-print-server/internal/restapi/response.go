package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cloud-print/server/internal/errs"
)

func writeSSEEvent(w http.ResponseWriter, ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
		return err
	}
	return nil
}

type apiResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, apiResponse{Code: "OK", Data: data})
}

func writeCreated(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, apiResponse{Code: "OK", Data: data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiResponse{Code: code, Message: message})
}

func writeAPIError(w http.ResponseWriter, err error) {
	if err == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unknown error")
		return
	}
	var apiErr *errs.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.HTTPStatus
		if status == 0 {
			status = http.StatusInternalServerError
		}
		writeError(w, status, string(apiErr.Code), apiErr.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}
