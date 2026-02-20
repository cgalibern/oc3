package feederhandlers

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/util/logkey"
)

func (a *Api) PostSystem(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostSystem")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	b, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Warn("request ReadAll", logkey.Error, err)
		return JSONProblemf(c, http.StatusBadRequest, "ReadAll: %s", err)
	}

	ctx := c.Request().Context()

	log.Info("Hset FeedSystemH")
	if _, err := a.Redis.HSet(ctx, cachekeys.FeedSystemH, nodeID, string(b)).Result(); err != nil {
		log.Error("Hset FeedSystemH", logkey.Error, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, cachekeys.FeedSystemPendingH, cachekeys.FeedSystemQ, nodeID); err != nil {
		log.Error("pushNotPending", "error", err)
		return JSONError(c)
	}

	return c.JSON(http.StatusAccepted, nil)
}
