package serverhandlers

import (
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetTag handles GET /tags/{tag_id}
func (a *Api) GetTag(c echo.Context, tagIdParam int) error {
	log := echolog.GetLogHandler(c, "GetTag")
	log.Info("called", logkey.TagID, tagIdParam)
	return a.handleGetTags(c, &tagIdParam)
}
