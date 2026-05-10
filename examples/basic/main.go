package main

import (
	"log"
	"net/http"

	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/server"
	"github.com/cerberauth/scimply/store"
)

func main() {
	reg := schema.NewRegistry()
	reg.RegisterDefaults()

	srv, err := server.New(
		server.WithStore(store.NewMemoryStore()),
		server.WithSchemaRegistry(reg),
		server.WithBasePath("/scim/v2"),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("SCIM 2.0 server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", srv))
}
