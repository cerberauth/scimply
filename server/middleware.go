package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cerberauth/scimply/protocol"
)

type middlewareFunc func(http.Handler) http.Handler

func chain(h http.Handler, middlewares ...middlewareFunc) http.Handler {

	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				detail := fmt.Sprintf("internal server error: %v", rec)
				protocol.NewSCIMError(http.StatusInternalServerError, "", detail).Write(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.authFunc == nil {
			next.ServeHTTP(w, r)
			return
		}
		ok, err := s.cfg.authFunc(r)
		if err != nil {
			protocol.NewSCIMError(http.StatusInternalServerError, "", "authentication error").Write(w)
			return
		}
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="SCIM"`)
			protocol.NewSCIMError(http.StatusUnauthorized, "", "unauthorized").Write(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.cfg.logger.Info("scim request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.statusCode),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if !protocol.IsAcceptable(r) {
			protocol.NewSCIMError(http.StatusNotAcceptable, "", "not acceptable: only application/scim+json and application/json are supported").Write(w)
			return
		}

		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			if ct != "" {
				if idx := strings.IndexByte(ct, ';'); idx >= 0 {
					ct = strings.TrimSpace(ct[:idx])
				}
				switch ct {
				case protocol.ContentTypeSCIM, protocol.ContentTypeJSON:

				default:
					protocol.NewSCIMError(http.StatusUnsupportedMediaType, "", fmt.Sprintf("unsupported Content-Type: %s", ct)).Write(w)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}
