package feederhandlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/feeder"
)

func (a *Api) GetVersion(c echo.Context) error {
	if SCHEMA.Info == nil {
		return JSONProblem(c, http.StatusInternalServerError, "invalid api schema")
	}
	return c.JSON(http.StatusOK, feeder.Version{Version: SCHEMA.Info.Version})
}
