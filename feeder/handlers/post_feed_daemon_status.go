package feederhandlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
)

func (a *Api) PostDaemonStatus(c echo.Context) error {
	nodeID, log := getNodeIDAndLogger(c, "PostDaemonStatus")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	mChange := make(map[string]struct{})

	mergeChanges := func(s string) {
		for _, v := range strings.Fields(s) {
			mChange[v] = struct{}{}
		}
	}

	body := c.Request().Body
	b, err := io.ReadAll(body)
	defer func() {
		if err := body.Close(); err != nil {
			log.Warn("request body Close", logError, err)
		}
	}()
	if err != nil {
		log.Warn("request ReadAll", logError, err)
		return JSONProblemf(c, http.StatusBadRequest, "ReadAll: %s", err)
	}
	postData := &feeder.PostDaemonStatus{}
	if err := json.Unmarshal(b, postData); err != nil {
		log.Debug("request Unmarshal", logError, err)
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	} else {
		mergeChanges(strings.Join(postData.Changes, " "))
	}
	if !strings.HasPrefix(postData.Version, "2.") && !strings.HasPrefix(postData.Version, "3.") {
		msg := fmt.Sprintf("unexpected version %s", postData.Version)
		log.Debug(msg)
		return JSONProblem(c, http.StatusBadRequest, msg)
	}
	ctx := c.Request().Context()
	log.Info("HSet FeedDaemonStatusH")
	if err := a.Redis.HSet(ctx, cachekeys.FeedDaemonStatusH, nodeID, string(b)).Err(); err != nil {
		log.Error("HSet FeedDaemonStatusH", logError, err)
		return JSONError(c)
	}
	if len(mChange) > 0 {
		// request contains changes, merge them to not yet applied changes
		log.Debug("HGet FeedDaemonStatusChangesH")
		redisChanges, err := a.Redis.HGet(ctx, cachekeys.FeedDaemonStatusChangesH, nodeID).Result()
		switch err {
		case nil:
			// merge existing changes
			mergeChanges(redisChanges)
		case redis.Nil:
			// no existing changes to merge
		default:
			log.Error("HGet FeedDaemonStatusChangesH", logError, err)
			return JSONError(c)
		}
		l := make([]string, len(mChange))
		i := 0
		for k := range mChange {
			l[i] = k
			i++
		}
		mergedChanges := strings.Join(l, " ")
		// push changes
		log.Debug("HSet FeedDaemonStatusChangesH", logChanges, mergedChanges)
		if err := a.Redis.HSet(ctx, cachekeys.FeedDaemonStatusChangesH, nodeID, mergedChanges).Err(); err != nil {
			log.Error("HSet FeedDaemonStatusChangesH", logChanges, mergedChanges, logError, err)
			return JSONError(c)
		}
	}
	if err := a.pushNotPending(ctx, log, cachekeys.FeedDaemonStatusPendingH, cachekeys.FeedDaemonStatusQ, nodeID); err != nil {
		log.Error("pushNotPending", logError, err)
		return JSONError(c)
	}

	clusterID := clusterIDFromContext(c)
	if clusterID != "" {
		if objects, err := a.getObjectConfigToFeed(ctx, clusterID); err != nil {
			log.Warn("getObjectConfigToFeed", logError, err)
		} else if len(objects) > 0 {
			if err := a.removeObjectConfigToFeed(ctx, clusterID); err != nil {
				log.Warn("removeObjectConfigToFeed", logError, err)
			}
			log.Info("accepted with detected missing object configs", logObjects, objects)
			return c.JSON(http.StatusAccepted, feeder.DaemonStatusAccepted{ObjectWithoutConfig: &objects})
		}
	}
	log.Info("accepted")
	return c.JSON(http.StatusAccepted, feeder.DaemonStatusAccepted{})
}
