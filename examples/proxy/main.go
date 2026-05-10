package main

import (
	"log"
	"net/http"
	"os"
	"time"

	scimclient "github.com/cerberauth/scimply/connector/scim"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
)

func main() {
	upstreamURL := os.Getenv("UPSTREAM_URL")
	if upstreamURL == "" {
		upstreamURL = "https://upstream.example.com/scim/v2"
	}
	upstreamToken := os.Getenv("UPSTREAM_TOKEN")
	if upstreamToken == "" {
		upstreamToken = "upstream-secret-token"
	}
	incomingToken := os.Getenv("SCIM_TOKEN")
	if incomingToken == "" {
		incomingToken = "my-incoming-token"
	}

	upstream, err := scimclient.New(
		scimclient.WithBaseURL(upstreamURL),
		scimclient.WithBearerToken(upstreamToken),
	)
	if err != nil {
		log.Fatal(err)
	}

	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	srv, err := server.New(
		server.WithStore(upstream),
		server.WithSchemaRegistry(reg),
		server.WithBasePath("/scim/v2"),
		server.WithBearerTokenAuth(func(token string) (bool, error) {
			return token == incomingToken, nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("SCIM proxy listening on :8080")
	s := &http.Server{
		Addr:              ":8080",
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(s.ListenAndServe())
}
