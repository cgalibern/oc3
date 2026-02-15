package feederhandlers

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/feeder"
)

type (
	Api struct {
		DB    *sql.DB
		Redis *redis.Client
		UI    bool

		// SyncTimeout is the timeout for synchronous api calls
		SyncTimeout time.Duration
	}
)

var (
	SCHEMA openapi3.T
)

func init() {
	if schema, err := feeder.GetSwagger(); err == nil {
		SCHEMA = *schema
	}
}

func getNodeIDAndLogger(c echo.Context, handler string) (string, *slog.Logger) {
	nodeID := nodeIDFromContext(c)
	log := getLogHandler(c, handler)
	if nodeID == "" {
		log.Debug("empty node id")
	}
	return nodeID, log
}
