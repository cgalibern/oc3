package feederhandlers

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
	"github.com/opensvc/oc3/util/logkey"
)

// PostInstanceResourceInfo will populate FeedInstanceResourceInfo <path>@<nodeID>@<clusterID>
// with posted instance resource information. The auth middleware has prepared nodeID and clusterID.
func (a *Api) PostInstanceResourceInfo(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostInstanceResourceInfo")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	var data feeder.PostInstanceResourceInfoJSONRequestBody
	keyH := cachekeys.FeedInstanceResourceInfoH
	keyQ := cachekeys.FeedInstanceResourceInfoQ
	keyPendingH := cachekeys.FeedInstanceResourceInfoPendingH

	clusterID := clusterIDFromContext(c)
	if clusterID == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define cluster id")
	}

	if err := c.Bind(&data); err != nil {
		log.Debug("Bind", logkey.Error, err)
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}
	if data.Path == "" {
		log.Debug("bad request: missing or empty instance path")
		return JSONProblem(c, http.StatusBadRequest, "missing or empty instance path")
	}
	log = log.With(logkey.Object, data.Path)
	b, err := json.Marshal(data)
	if err != nil {
		log.Warn("Marshal", logkey.Error, err)
		return JSONError(c)
	}
	ctx := c.Request().Context()
	idx := data.Path + "@" + nodeID + "@" + clusterID
	log.Info("HSet keyH")
	if err := a.Redis.HSet(ctx, keyH, idx, b).Err(); err != nil {
		log.Error("HSet keyH", logkey.Error, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, keyPendingH, keyQ, idx); err != nil {
		log.Error("pushNotPending", logkey.Error, err)
		return JSONError(c)
	}
	return c.JSON(http.StatusAccepted, nil)
}
