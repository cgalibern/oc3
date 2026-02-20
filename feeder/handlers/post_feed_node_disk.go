package feederhandlers

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
	"github.com/opensvc/oc3/util/logkey"
)

// PostNodeDisk will populate FeedNodeDiskH <nodename>@<nodeID>@<clusterID>
// with posted disk, auth middleware has prepared nodeID, clusterID, and nodename.
func (a *Api) PostNodeDisk(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostNodeDisk")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	keyH := cachekeys.FeedNodeDiskH
	keyQ := cachekeys.FeedNodeDiskQ
	keyPendingH := cachekeys.FeedNodeDiskPendingH

	nodename := nodenameFromContext(c)
	if nodename == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define nodename")
	}
	clusterID := clusterIDFromContext(c)
	if clusterID == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define cluster id")
	}
	var payload feeder.PostNodeDiskJSONRequestBody
	if err := c.Bind(&payload); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}
	b, err := json.Marshal(payload)
	if err != nil {
		log.Warn("Marshal", logkey.Error, err)
		return JSONError(c)
	}
	ctx := c.Request().Context()
	idx := nodename + "@" + nodeID + "@" + clusterID
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
