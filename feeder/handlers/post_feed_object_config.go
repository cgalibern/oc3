package feederhandlers

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
	"github.com/opensvc/oc3/util/logkey"
)

// PostObjectConfig will populate FeedObjectConfigH <path>@<nodeID>@<clusterID>
// with posted object config. The auth middleware has prepared nodeID and clusterID.
func (a *Api) PostObjectConfig(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostObjectConfig")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	keyH := cachekeys.FeedObjectConfigH
	keyQ := cachekeys.FeedObjectConfigQ
	keyPendingH := cachekeys.FeedObjectConfigPendingH

	clusterID := clusterIDFromContext(c)
	if clusterID == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define cluster id")
	}
	var payload feeder.PostObjectConfigJSONRequestBody
	if err := c.Bind(&payload); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}
	if payload.Path == "" {
		log.Debug("missing or empty object path")
		return JSONProblem(c, http.StatusBadRequest, "missing or empty object path")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		log.Error("json encode config", logkey.Error, err)
		return JSONError(c)
	}
	ctx := c.Request().Context()
	idx := payload.Path + "@" + nodeID + "@" + clusterID
	log.Info("Hset keyH")
	if err := a.Redis.HSet(ctx, keyH, idx, b).Err(); err != nil {
		log.Error("Hset keyH", logkey.Error, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, keyPendingH, keyQ, idx); err != nil {
		log.Error("pushNotPending", logkey.Error, err)
		return JSONError(c)
	}
	return c.JSON(http.StatusAccepted, nil)
}
