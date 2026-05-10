package protocol

import (
	"net/http"
	"strings"
)

const (
	ContentTypeSCIM = "application/scim+json"
	ContentTypeJSON = "application/json"

	anyType    = "*/*"
	anyAppType = "application/*"
)

func NegotiateContentType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return ContentTypeSCIM
	}
	for _, part := range strings.Split(accept, ",") {
		mt := strings.TrimSpace(part)
		if idx := strings.IndexByte(mt, ';'); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		switch mt {
		case ContentTypeSCIM:
			return ContentTypeSCIM
		case ContentTypeJSON:
			return ContentTypeJSON
		case anyType, anyAppType:
			return ContentTypeSCIM
		}
	}
	return ContentTypeSCIM
}

func SetContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentTypeSCIM)
}

func IsAcceptable(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return true
	}
	for _, part := range strings.Split(accept, ",") {
		mt := strings.TrimSpace(part)
		if idx := strings.IndexByte(mt, ';'); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		switch mt {
		case ContentTypeSCIM, ContentTypeJSON, anyType, anyAppType:
			return true
		}
	}
	return false
}
