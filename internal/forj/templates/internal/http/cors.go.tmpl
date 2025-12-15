package http

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/goforj/env"
	"net/http"
	"strings"
)

// registerCors registers the CORS middleware.
func (s *Server) registerCors(e *echo.Echo) {
	// cors
	corsAllow := env.Get("HTTP_CORS_ALLOW_ENDPOINTS", "")
	endpoints := strings.Split(corsAllow, ",")

	// remove empty strings
	for i := len(endpoints) - 1; i >= 0; i-- {
		if endpoints[i] == "" {
			endpoints = append(endpoints[:i], endpoints[i+1:]...)
		}
	}

	if len(endpoints) == 0 && env.IsAppEnvDev() {
		// If no endpoints are specified and the app is in dev mode, allow all origins
		endpoints = []string{"http://localhost:8080"}
	}

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: endpoints,
		AllowMethods: []string{
			http.MethodOptions,
			http.MethodGet,
			http.MethodHead,
			http.MethodPut,
			http.MethodPatch,
			http.MethodPost,
			http.MethodDelete,
		},
		AllowCredentials: true,
	}))
}
