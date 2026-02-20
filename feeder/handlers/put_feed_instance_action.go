package feederhandlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
	"github.com/opensvc/oc3/util/logkey"
)

/*
{
    "uuid": "ea9a8373-3dda-4fe7-8c4b-08f5290c6c8b",
    "path": "foo",
    "action": "thaw",
    "begin": "2026-01-21T18:00:00.357669+01:00",
    "end": "2026-01-21T18:05:00.357669+01:00",
    "cron": false,
    "session_uuid": "7f2df7b2-8a4a-4bc1-9a8b-03acffaacd45",
    "actionlogfile": "/var/tmp/opensvc/foo.freezeupdfl98l",
    "status": "ok"
}
*/

// PutInstanceActionEnd handles PUT /feed/action
func (a *Api) PutInstanceActionEnd(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PutInstanceActionEnd")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	keyH := cachekeys.FeedInstanceActionH
	keyQ := cachekeys.FeedInstanceActionQ
	keyPendingH := cachekeys.FeedInstanceActionPendingH

	ClusterID := clusterIDFromContext(c)
	if ClusterID == "" {
		msg := "refused: authenticated node doesn't define cluster id"
		log.Info(msg)
		return JSONProblem(c, http.StatusConflict, msg)
	}

	var payload feeder.PutInstanceActionEndJSONRequestBody
	if err := c.Bind(&payload); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Error("Marshall", logkey.Error, err)
		return JSONError(c)
	}

	ctx := c.Request().Context()

	idx := fmt.Sprintf("%s@%s@%s:%s", payload.Path, nodeID, ClusterID, payload.Uuid)

	log.Debug("HSet keyH")
	if _, err := a.Redis.HSet(ctx, keyH, idx, b).Result(); err != nil {
		log.Error("HSet keyH", logkey.Error, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, keyPendingH, keyQ, idx); err != nil {
		log.Error("pushNotPending", logkey.Error, err)
		return JSONError(c)
	}

	log.Debug("action end accepted")
	return c.JSON(http.StatusAccepted, nil)
}
