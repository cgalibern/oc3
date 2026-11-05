package serverhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAgentMiddleware(t *testing.T) {
	cases := map[string]struct {
		route, target, mode string
		want                int
	}{
		"node on an agent route":        {"/api/agent/tags/:name", "/api/agent/tags/foo", AuthModeNode, http.StatusOK},
		"user on an agent route":        {"/api/agent/tags/:name", "/api/agent/tags/foo", AuthModeUser, http.StatusForbidden},
		"encoded path matches no route": {"/api/agent/tags", "/api/%61gent/tags", AuthModeUser, http.StatusNotFound},
		"no mode on an agent route":     {"/api/agent/compliance/data", "/api/agent/compliance/data", "", http.StatusForbidden},
		"user on another route":         {"/api/tags", "/api/tags", AuthModeUser, http.StatusOK},
		"user on a route like prefix":   {"/api/agents", "/api/agents", AuthModeUser, http.StatusOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := echo.New()
			e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					if tc.mode != "" {
						c.Set(XAuthMode, tc.mode)
					}
					return next(c)
				}
			})
			e.Use(AgentMiddleware("/api/agent"))
			e.GET(tc.route, func(c echo.Context) error { return c.NoContent(http.StatusOK) })
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if rec.Code != tc.want {
				t.Fatalf("got status %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
