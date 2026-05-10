package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cerberauth/scimply/audit"
	"github.com/cerberauth/scimply/protocol"
	"github.com/cerberauth/scimply/schema"
)

type Server struct {
	cfg config
}

func New(opts ...Option) (*Server, error) {
	cfg := config{
		defaultPageSize: 100,
		maxPageSize:     1000,
		maxBulkOps:      1000,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.store == nil {
		return nil, errors.New("scimply/server: store is required; use WithStore()")
	}
	if cfg.registry == nil {
		return nil, errors.New("scimply/server: schema registry is required; use WithSchemaRegistry()")
	}
	if cfg.auditLogger == nil {
		cfg.auditLogger = audit.Noop()
	}
	if cfg.logger == nil {
		cfg.logger = slog.Default()
	}
	if cfg.spConfig == nil {
		cfg.spConfig = defaultServiceProviderConfig(cfg)
	}

	return &Server{cfg: cfg}, nil
}

func defaultServiceProviderConfig(cfg config) *schema.ServiceProviderConfig {
	return &schema.ServiceProviderConfig{
		Patch:          schema.Supported{Supported: true},
		Bulk:           schema.BulkConfig{Supported: true, MaxOperations: cfg.maxBulkOps, MaxPayloadSize: 1048576},
		Filter:         schema.FilterConfig{Supported: true, MaxResults: cfg.maxPageSize},
		ChangePassword: schema.Supported{Supported: false},
		Sort:           schema.Supported{Supported: true},
		ETag:           schema.Supported{Supported: true},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := chain(
		http.HandlerFunc(s.route),
		recoveryMiddleware,
		contentTypeMiddleware,
		s.authMiddleware,
		s.loggingMiddleware,
	)
	h.ServeHTTP(w, r)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if s.cfg.basePath != "" {
		stripped := strings.TrimPrefix(path, s.cfg.basePath)
		if stripped == path {

			protocol.NewSCIMError(http.StatusNotFound, "", "not found").Write(w)
			return
		}
		path = stripped
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	first := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}

	switch strings.ToLower(first) {
	case "":
		protocol.NewSCIMError(http.StatusNotFound, "", "not found").Write(w)

	case "serviceproviderconfig":
		if r.Method != http.MethodGet {
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
			return
		}
		s.handleServiceProviderConfig(w, r)

	case "resourcetypes":
		if r.Method != http.MethodGet {
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
			return
		}
		s.handleResourceTypes(w, r)

	case schemasKey:
		if r.Method != http.MethodGet {
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
			return
		}
		s.handleSchemas(w, r)

	case "bulk":
		if r.Method != http.MethodPost {
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
			return
		}
		s.handleBulk(w, r)

	default:
		s.routeResource(w, r, first, rest)
	}
}

func (s *Server) routeResource(w http.ResponseWriter, r *http.Request, endpoint, rest string) {
	rt, ok := s.cfg.registry.ResourceTypeByEndpoint("/" + endpoint)
	if !ok {
		protocol.NewSCIMError(http.StatusNotFound, protocol.SCIMType(""),
			fmt.Sprintf("unknown resource type endpoint: /%s", endpoint)).Write(w)
		return
	}
	resourceType := rt.Name

	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			s.handleList(w, r, resourceType)
		case http.MethodPost:
			s.handleCreate(w, r, resourceType)
		default:
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
		}
		return
	}

	idParts := strings.SplitN(rest, "/", 2)
	second := idParts[0]

	if second == ".search" {
		if r.Method != http.MethodPost {
			protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
			return
		}
		s.handleList(w, r, resourceType)
		return
	}

	id := second
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, resourceType, id)
	case http.MethodPut:
		s.handleReplace(w, r, resourceType, id)
	case http.MethodPatch:
		s.handlePatch(w, r, resourceType, id)
	case http.MethodDelete:
		s.handleDelete(w, r, resourceType, id)
	default:
		protocol.NewSCIMError(http.StatusMethodNotAllowed, "", "method not allowed").Write(w)
	}
}

func (s *Server) resourceLocation(r *http.Request, resourceType, id string) string {
	rt, ok := s.cfg.registry.ResourceTypeByName(resourceType)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s://%s%s%s/%s", scheme(r), r.Host, s.cfg.basePath, rt.Endpoint, id)
}

func etagFromVersion(version string) string {
	if version == "" {
		return ""
	}
	return fmt.Sprintf(`W/"%s"`, version)
}
