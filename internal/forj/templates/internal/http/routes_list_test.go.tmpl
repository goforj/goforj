package http

import (
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestGetRoutesListMergesMethodsAndMiddlewares(t *testing.T) {
	s := &Server{
		groups: []RouteGroup{
			NewRouteGroup(
				"/api",
				[]Route{
					NewRoute("GET", "/users", testHandler, mwRoute),
					NewRoute("POST", "/users", testHandler, mwRoute),
				},
				mwGroup,
			),
		},
	}

	entries := s.getRoutesList()
	if assert.Len(t, entries, 1) {
		entry := entries[0]
		assert.Equal(t, "/api/users", entry.Path)
		assert.ElementsMatch(t, []string{"GET", "POST"}, entry.Methods)
		assert.Equal(t, "gm.testHandler", entry.Handler)
		assert.Equal(t, []string{"http.mwGroup", "http.mwRoute"}, entry.Middlewares)
	}
}

func TestRouteEntriesToRowsColorsEachMethod(t *testing.T) {
	entry := &routeEntry{
		Path:        "/multi",
		Handler:     "gm.testHandler",
		Methods:     []string{"POST", "GET"},
		Middlewares: []string{"http.mwGroup"},
	}

	rows := routeEntriesToRows([]*routeEntry{entry}, middlewareRenderConfig{})
	if assert.Len(t, rows, 1) && assert.Len(t, rows[0], 4) {
		// Strip ANSI color sequences before asserting the content
		methodCell := stripANSI(rows[0][1])
		assert.Equal(t, "GET, POST", methodCell)
	}
}

func TestRouteEntriesToRowsUsesShortcodes(t *testing.T) {
	entry := &routeEntry{
		Path:        "/multi",
		Handler:     "gm.testHandler",
		Methods:     []string{"GET"},
		Middlewares: []string{"http.mwGroup", "http.mwRoute"},
	}

	codeToName, nameToCode := buildMiddlewareShortcodes([]*routeEntry{entry})
	cfg := middlewareRenderConfig{useShortcodes: true, nameToCode: nameToCode}

	rows := routeEntriesToRows([]*routeEntry{entry}, cfg)
	if assert.Len(t, rows, 1) && assert.Len(t, rows[0], 4) {
		mwCell := stripANSI(rows[0][3])
		assert.Equal(t, nameToCode["http.mwGroup"]+", "+nameToCode["http.mwRoute"], mwCell)
		assert.Equal(t, "http.mwGroup", codeToName[nameToCode["http.mwGroup"]])
	}
}

func TestShouldUseMiddlewareShortcodes(t *testing.T) {
	short := []*routeEntry{
		{Middlewares: []string{"mw1", "mw2"}},
	}
	assert.False(t, shouldUseMiddlewareShortcodes(short))

	long := []*routeEntry{
		{Middlewares: []string{strings.Repeat("mw-long-name", 10)}},
	}
	assert.True(t, shouldUseMiddlewareShortcodes(long))
}

func TestBuildMiddlewareShortcodesDeterministic(t *testing.T) {
	entries := []*routeEntry{
		{Middlewares: []string{"mw1", "mw2"}},
		{Middlewares: []string{"mw2", "mw3"}},
	}

	codeToName, nameToCode := buildMiddlewareShortcodes(entries)
	assert.Len(t, codeToName, 3)
	assert.Len(t, nameToCode, 3)

	assert.Equal(t, codeToName[nameToCode["mw1"]], "mw1")
	assert.Equal(t, codeToName[nameToCode["mw2"]], "mw2")
}

func TestFriendlyMiddlewareCodeReadable(t *testing.T) {
	assert.Equal(t, "JWTM.M", friendlyMiddlewareCode("JWTManager.Middleware"))
	assert.Equal(t, "M.GWC", friendlyMiddlewareCode("middleware.GzipWithConfig"))
	assert.Equal(t, "H.G", friendlyMiddlewareCode("http.mwGroup"))
}

// Helpers

func testHandler(c echo.Context) error { return nil }

func mwGroup(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error { return next(c) }
}

func mwRoute(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error { return next(c) }
}

func stripANSI(s string) string {
	re := regexp.MustCompile("\x1b\\[[0-9;]*m")
	return re.ReplaceAllString(s, "")
}
