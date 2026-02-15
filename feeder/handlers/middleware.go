package feederhandlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"

	"github.com/opensvc/oc3/xauth"
)

const (
	XClusterID = "XClusterID"
	XNodeID    = "XNodeID"
	XNodename  = "XNodename"
	XLogger    = "XLogger"
)

// AuthMiddleware returns auth middleware that authenticates requests from strategies.
func AuthMiddleware(strategies union.Union) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			_, user, err := strategies.AuthenticateRequest(c.Request())
			if err != nil {
				code := http.StatusUnauthorized
				return JSONProblem(c, code, err.Error())
			} else if user == nil {
				return next(c)
			}
			ext := user.GetExtensions()
			if nodeID := ext.Get(xauth.XNodeID); nodeID != "" {
				// request user is a node, sets node ID in echo context
				c.Set(XNodeID, nodeID)
				log := getLog(c).With(logNodeID, nodeID)

				if nodename := ext.Get(xauth.XNodename); nodename != "" {
					c.Set(XNodename, nodename)
					log = log.With(logNodename, nodename)
				}

				if clusterID := ext.Get(xauth.XClusterID); clusterID != "" {
					// request user is a node with a cluster ID, sets cluster ID in echo context
					c.Set(XClusterID, clusterID)
					log = log.With(logClusterID, clusterID)
				} else {
					c.Set(XLogger, getLog(c).With(logNodeID, nodeID, logClusterID, clusterID))
				}
				c.Set(XLogger, log)
			}

			c.Set("user", user)
			return next(c)
		}
	}
}

// nodeIDFromContext returns the nodeID from context or zero string
// if not found.
func nodeIDFromContext(c echo.Context) string {
	nodeID, ok := c.Get(XNodeID).(string)
	if ok {
		return nodeID
	}
	return ""
}

// nodenameFromContext returns the nodename from context or zero string
// if not found.
func nodenameFromContext(c echo.Context) string {
	nodename, ok := c.Get(XNodename).(string)
	if ok {
		return nodename
	}
	return ""
}

// clusterIDFromContext returns the clusterID from context or zero string
// if not found.
func clusterIDFromContext(c echo.Context) string {
	s, ok := c.Get(XClusterID).(string)
	if ok {
		return s
	}
	return ""
}
