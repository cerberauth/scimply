package v1

import (
	"encoding/json"
	"net/http"
)

const ServiceProviderConfigsPath = "/ServiceProviderConfigs"

type V1Error struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type V1ErrorResponse struct {
	Schemas []string  `json:"schemas"`
	Errors  []V1Error `json:"Errors"`
}

func WriteV1Error(w http.ResponseWriter, statusCode int, description string) {
	resp := V1ErrorResponse{
		Schemas: []string{CoreSchemaURI},
		Errors: []V1Error{
			{
				Code:        http.StatusText(statusCode),
				Description: description,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

func WrapEndpoint(inner http.Handler, resourceTypeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		inner.ServeHTTP(w, r)
	})
}

func FilterV1ToV2(filter string) string {
	return filter
}
