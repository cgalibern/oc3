package echolog

import (
	"log/slog"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/logkey"
)

const (
	LoggerKey = "logger"
)

func GetLog(c echo.Context) *slog.Logger {
	if l, ok := c.Get(LoggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func SetLog(c echo.Context, l *slog.Logger) {
	c.Set(LoggerKey, l)
}

func GetLogHandler(c echo.Context, handler string) *slog.Logger {
	return GetLog(c).With(logkey.Handler, handler)
}
