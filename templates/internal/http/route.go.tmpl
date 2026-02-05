package http

import "github.com/labstack/echo/v4"

// NewRoute creates a new route with the specified method, route, handler, and middlewares.
func NewRoute(
	method string,
	route string,
	handler echo.HandlerFunc,
	middlewares ...echo.MiddlewareFunc,
) Route {
	return Route{
		method:      method,
		route:       route,
		handler:     handler,
		middlewares: middlewares,
	}
}

// Route represents a single route in the application.
type Route struct {
	method      string
	route       string
	handler     echo.HandlerFunc
	middlewares []echo.MiddlewareFunc
}

// Method returns the HTTP method
func (r *Route) Method() string {
	return r.method
}

// Path returns the path of the route
func (r *Route) Path() string {
	return r.route
}

// Handler returns the handler method
func (r *Route) Handler() echo.HandlerFunc {
	return r.handler
}

// Middlewares returns the middlewares
// to be applied to the route
func (r *Route) Middlewares() []echo.MiddlewareFunc {
	if len(r.middlewares) > 0 {
		return r.middlewares
	}

	return []echo.MiddlewareFunc{}
}

// NewRouteGroup wraps routes and their accompanied middleware
func NewRouteGroup(
	prefix string,
	routes []Route,
	middlewares ...echo.MiddlewareFunc,
) RouteGroup {
	return RouteGroup{
		routePrefix: prefix,
		routes:      routes,
		middlewares: middlewares,
	}
}

// RouteGroup represents a group of routes
type RouteGroup struct {
	routePrefix string
	routes      []Route
	middlewares []echo.MiddlewareFunc
}

// RoutePrefix returns the prefix of the route group
func (c *RouteGroup) RoutePrefix() string {
	return c.routePrefix
}

// Routes returns the routes in the group
func (c *RouteGroup) Routes() []Route {
	return c.routes
}

// Middlewares returns the middlewares for the group
func (c *RouteGroup) Middlewares() []echo.MiddlewareFunc {
	return c.middlewares
}
