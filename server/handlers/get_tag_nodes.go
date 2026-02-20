package serverhandlers

import (
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetTagNodes handles GET /tags/{tag_id}/nodes
func (a *Api) GetTagNodes(c echo.Context, tagIdParam int) error {
	log := echolog.GetLogHandler(c, "GetTagNodes")

	log.Info("called", logkey.TagID, tagIdParam)

	// TODO

	return c.JSON(200, []interface{}{})
}
