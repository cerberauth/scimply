package protocol

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, body interface{}) {
	SetContentType(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteResource(w http.ResponseWriter, r *http.Request, body interface{}, location, etag string) {
	if location != "" {
		w.Header().Set("Location", location)
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	ct := NegotiateContentType(r)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteCreated(w http.ResponseWriter, r *http.Request, body interface{}, location, etag string) {
	if location != "" {
		w.Header().Set("Location", location)
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	ct := NegotiateContentType(r)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
