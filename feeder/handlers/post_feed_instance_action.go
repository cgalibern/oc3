package feederhandlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
)

/*
{
  "action": "thaw",
  "argv": [
    "foo",
    "thaw",
    "--local"
  ],
  "begin": "2026-01-21T18:00:00.357669+01:00",
  "cron": false,
  "path": "foo",
  "session_uuid": "b9d795bc-498e-4c20-aada-9feec2eaa947",
  "version": "2.1-1977"
}
*/

// PostInstanceAction handles POST /action/begin
func (a *Api) PostInstanceAction(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostInstanceAction")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	keyH := cachekeys.FeedInstanceActionH
	keyQ := cachekeys.FeedInstanceActionQ
	keyPendingH := cachekeys.FeedInstanceActionPendingH

	ClusterID := clusterIDFromContext(c)
	if ClusterID == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define cluster id")
	}

	var payload feeder.PostInstanceActionJSONRequestBody
	if err := c.Bind(&payload); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}

	if !strings.HasPrefix(payload.Version, "2.") && !strings.HasPrefix(payload.Version, "3.") {
		log.Debug("unsupported data client version")
		return JSONProblemf(c, http.StatusBadRequest, "unsupported data client version: %s", payload.Version)
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Error("json encode body", logError, err)
		return JSONError(c)
	}

	ctx := c.Request().Context()

	uuid := uuid.New().String()
	idx := fmt.Sprintf("%s@%s@%s:%s", payload.Path, nodeID, ClusterID, uuid)

	log.Debug("Hset FeedInstanceActionH")
	if _, err := a.Redis.HSet(ctx, keyH, idx, b).Result(); err != nil {
		log.Error("Hset FeedInstanceActionH", logError, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, keyPendingH, keyQ, idx); err != nil {
		log.Error("pushNotPending", "error", err)
		return JSONError(c)
	}

	log.Debug("accepted")
	return c.JSON(http.StatusAccepted, feeder.ActionRequestAccepted{Uuid: uuid})
}
