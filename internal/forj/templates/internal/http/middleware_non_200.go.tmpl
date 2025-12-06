package http

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// responseBodyWriter is a custom http.ResponseWriter that captures the response body.
type responseBodyWriter struct {
	http.ResponseWriter
	status int
	body   *strings.Builder
}

// WriteHeader creates a new response body writer.
func (w *responseBodyWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Write captures the response body for non-200 statuses.
func (w *responseBodyWriter) Write(b []byte) (int, error) {
	if w.status != http.StatusOK {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// logNon200ResponseBodyMiddleware captures response body for non-200 statuses.
func (s *Server) logNon200ResponseBodyMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Wrap the underlying http.ResponseWriter, not echo.ResponseWriter
			originalWriter := c.Response().Writer
			writer := &responseBodyWriter{
				ResponseWriter: originalWriter,
				body:           new(strings.Builder),
				status:         http.StatusOK,
			}
			c.Response().Writer = writer

			err := next(c)

			if writer.status != http.StatusOK && writer.body.Len() > 0 {
				s.logger.Info().
					Any("status", writer.status).
					Str("body", writer.body.String()).
					Msg("Non-200 HTTP Response")
			}

			return err
		}
	}
}
