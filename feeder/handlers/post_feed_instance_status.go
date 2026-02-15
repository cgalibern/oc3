package feederhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cachekeys"
	"github.com/opensvc/oc3/feeder"
)

func (a *Api) PostInstanceStatus(c echo.Context, params feeder.PostInstanceStatusParams) error {
	nodeID, log := getNodeIDAndLogger(c, "PostInstanceStatus")
	if nodeID == "" {
		return JSONNodeAuthProblem(c)
	}

	var (
		keyH        = cachekeys.FeedInstanceStatusH
		keyQ        = cachekeys.FeedInstanceStatusQ
		keyPendingH = cachekeys.FeedInstanceStatusPendingH

		syncMode bool
		pubsub   *redis.PubSub
		doneC    chan any

		// timeout is the default timeout for api calls
		timeout = 2 * time.Second
	)

	clusterID := clusterIDFromContext(c)
	if clusterID == "" {
		return JSONProblemf(c, http.StatusConflict, "refused: authenticated node doesn't define cluster id")
	}

	var payload feeder.PostInstanceStatusJSONRequestBody
	if err := c.Bind(&payload); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}

	if params.Sync != nil && *params.Sync {
		syncMode = true
		if a.SyncTimeout > 0 {
			timeout = a.SyncTimeout
		}
	}

	if !strings.HasPrefix(payload.Version, "2.") {
		log.Error(fmt.Sprintf("unexpected version %s", payload.Version))
		return JSONProblemf(c, http.StatusBadRequest, "unsupported data client version: %s", payload.Version)
	}

	if payload.Path == "" {
		return JSONProblem(c, http.StatusBadRequest, "missing or empty instance path")
	}
	log = log.With(logObject, payload.Path)

	b, err := json.Marshal(payload)
	if err != nil {
		return JSONProblemf(c, http.StatusInternalServerError, "json encode request body: %s", err)
	}

	id := fmt.Sprintf("%s@%s@%s", payload.Path, nodeID, clusterID)

	ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
	defer cancel()

	if syncMode {
		pubsub = a.Redis.Subscribe(ctx, cachekeys.FeedInstanceStatusP)
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Error("redis pubsub Close", logError, err)
			}
		}()
		if i, err := pubsub.Receive(ctx); err != nil {
			log.Error("redis pubsub Receive", logError, err)
			return JSONError(c)
		} else {
			switch i.(type) {
			case *redis.Subscription:
			default:
				log.Error(fmt.Sprintf("redis pubsub Receive unexpected message type %T", i))
				return JSONError(c)
			}
		}
		doneC = make(chan any)
		go func() {
			// Wait for confirmation that the data has been stored
			for {
				// Ensure return on ctx done
				select {
				case <-ctx.Done():
					return
				default:
				}

				if i, err := pubsub.Receive(ctx); err != nil {
					log.Debug("redis pubsub Receive", logError, err)
					return
				} else {
					switch m := i.(type) {
					case *redis.Message:
						if m == nil {
							continue
						} else if m.Payload == id {
							log.Debug("redis pubsub Received matching id")
							doneC <- nil
							return
						}
					}
				}
			}
		}()
	}

	// Store data in Redis hash with generated ID as key
	log.Debug("Hset keyH")
	if _, err := a.Redis.HSet(ctx, keyH, id, b).Result(); err != nil {
		log.Error("Hset keyH", logError, err)
		return JSONError(c)
	}

	if err := a.pushNotPending(ctx, log, keyPendingH, keyQ, id); err != nil {
		log.Error("pushNotPending", logError, err)
		return JSONError(c)
	}

	if syncMode {
		select {
		case <-doneC:
			return c.JSON(http.StatusOK, nil)
		case <-ctx.Done():
			return c.JSON(http.StatusAccepted, nil)
		}
	} else {
		return c.JSON(http.StatusAccepted, nil)
	}
}
