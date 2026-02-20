package serverhandlers

import (
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetNodeComplianceLogs handles GET /nodes/{node_id}/compliance/logs
func (a *Api) GetNodeComplianceLogs(c echo.Context, nodeId string) error {
	log := echolog.GetLogHandler(c, "GetNodeComplianceLogs")

	log.Info("called", logkey.NodeID, nodeId)

	// TODO

	return c.JSON(200, []any{})
}
