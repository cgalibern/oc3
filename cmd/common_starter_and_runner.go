package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo-contrib/pprof"
	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"
	"github.com/spf13/viper"

	"github.com/opensvc/oc3/xauth"
)

const (
	pathApi    = "/api"
	pathSpec   = "openapi.json"
	pathPprof  = "/pprof"
	pathMetric = "/metrics"
)

type (
	Sectioner interface {
		Section() string
	}

	apiRegister interface {
		apiRegister(e *echo.Echo)
	}

	docMiddlerwarer interface {
		docMiddleware() echo.MiddlewareFunc
	}

	authMiddlewarer interface {
		authMiddleware(publicPath, publicPrefix []string) echo.MiddlewareFunc
	}
)

func run(i Sectioner) error {
	if ok, errC := start(i); ok {
		slog.Info("started")
		return <-errC
	}
	return fmt.Errorf("start failed")
}

func start(i Sectioner) (bool, <-chan error) {
	var needRun bool
	errC := make(chan error, 1)
	section := i.Section()

	// get enabled features
	enableUI := viper.GetBool(section + ".ui.enable")
	enableMetrics := viper.GetBool(section + ".metrics.enable")
	enablePprofNet := viper.GetBool(section + ".pprof.net.enable")
	enablePprofUx := viper.GetBool(section + ".pprof.ux.enable")

	// define public paths
	publicPath := []string{}
	publicPrefix := []string{}
	if _, ok := i.(apiRegister); ok {
		needRun = true
		publicPath = append(publicPath, pathApi+"/version")

		if enableUI {
			publicPath = append(publicPath, pathApi)
			for _, p := range []string{"", pathSpec, "swagger-ui.css", "swagger-ui-bundle.js", "swagger-ui-standalone-preset.js"} {
				publicPath = append(publicPath, pathApi+"/"+p)
			}
		}
	} else {
		enableUI = false
	}

	if enableMetrics {
		needRun = true
		publicPath = append(publicPath, pathMetric)
	}
	if enablePprofNet {
		needRun = true
		publicPrefix = append(publicPrefix, pathPprof)
	}
	if enablePprofUx {
		needRun = true
	}
	if !needRun {
		slog.Debug(fmt.Sprintf("api, metrics, profile not needed for %s", section))
		return false, nil
	}
	slog.Info(fmt.Sprintf("public paths: %s", strings.Join(publicPath, ", ")))
	slog.Info(fmt.Sprintf("public path prefixes: %s", strings.Join(publicPrefix, ", ")))

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	if a, ok := i.(authMiddlewarer); ok {
		e.Use(a.authMiddleware(publicPath, publicPrefix))
	}

	if a, ok := i.(apiRegister); ok {
		slog.Info(fmt.Sprintf("add handler for openapi: %s", pathApi))
		a.apiRegister(e)
	}

	if enablePprofUx {
		if p := viper.GetString(section + ".pprof.ux.socket"); p != "" {
			slog.Info(fmt.Sprintf("add handler for profiling ux.socket: %s", p))
			if err := pprofUx(p); err != nil {
				errC <- err
				return true, errC
			}
		}
	}
	if enablePprofNet {
		pprofRegister(e)
	}

	if enableMetrics {
		metricsRegister(e)
	}

	if enableUI {
		if a, ok := i.(docMiddlerwarer); ok {
			registerDoc(e, a.docMiddleware())
		}
	}

	running := make(chan bool)
	go func() {
		addr := viper.GetString(section + ".addr")
		slog.Info("listen on " + addr)
		running <- true
		errC <- e.Start(addr)
	}()
	return <-running, errC
}

func pprofRegister(e *echo.Echo) {
	// TODO: move to authenticated path
	slog.Info(fmt.Sprintf("add handler for profiling inet: %s", pathPprof))
	pprof.Register(e, pathPprof)
	e.GET(pathPprof, func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, endingSlash(relPath(pathPprof)))
	})
}

func metricsRegister(e *echo.Echo) {
	// TODO: move to authenticated path
	slog.Info(fmt.Sprintf("add handler for metrics: %s", pathMetric))
	e.Use(echoprometheus.NewMiddleware("oc3_feeder"))
	e.GET(pathMetric, echoprometheus.NewHandler())
}

func registerDoc(e *echo.Echo, m echo.MiddlewareFunc) {
	slog.Info(fmt.Sprintf("add handler for documentation ui: %s", pathApi))
	g := e.Group(pathApi)
	g.Use(m)
	e.GET(pathApi, func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, endingSlash(relPath(pathApi)))
	})
}

// AuthMiddleware returns auth middleware that authenticate requests from strategies.
func AuthMiddleware(strategies union.Union) echo.MiddlewareFunc {
	const XUserEmail = "XUserEmail"

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			_, user, err := strategies.AuthenticateRequest(c.Request())

			if err != nil {
				return c.JSON(http.StatusUnauthorized, nil)
			}

			ext := user.GetExtensions()
			if userEmail := ext.Get(xauth.XUserEmail); userEmail != "" {
				c.Set(XUserEmail, userEmail)
			}

			groups := user.GetGroups()
			c.Set("groups", groups)
			c.Set("user", user)

			return next(c)
		}
	}
}

func endingSlash(s string) string { return strings.TrimSuffix(s, "/") + "/" }

func relPath(s string) string { return "./" + strings.TrimPrefix(s, "/") }
