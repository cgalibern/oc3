package feederhandlers

import (
	"log/slog"

	"github.com/labstack/echo/v4"
)

var (
	defaultLogger = slog.Default()
)

const (
	// logHandler represents the key used to log the handler name in structured logs.
	logHandler = "handler"

	// logError represents the key used to log an error in structured logs.
	logError = "error"

	// logNodeID represents the key used to log the node identifier in structured logs.
	logNodeID = "nodeID"

	// logNodename represents the key used to log the nodename in structured logs.
	logNodename = "nodename"

	// logClusterID represents the key used to log the cluster identifier in structured logs.
	logClusterID = "clusterID"

	// logChanges represents the key used to log the changes in structured logs.
	logChanges = "changes"

	// logObjects represents the key used to log the objects in structured logs.
	logObjects = "objects"

	// logObject represents the key used to log the object in structured logs.
	logObject = "object"
)

func getLog(c echo.Context) *slog.Logger {
	if l, ok := c.Get(XLogger).(*slog.Logger); ok {
		return l
	}
	return defaultLogger
}

func getLogHandler(c echo.Context, handler string) *slog.Logger {
	return getLog(c).With(logHandler, handler)
}
