package serverhandlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetNodeComplianceRulesets handles GET /nodes/{node_id}/compliance/rulesets
func (a *Api) GetNodeComplianceRulesets(c echo.Context, nodeId string) error {
	log := echolog.GetLogHandler(c, "GetNodeComplianceRulesets")
	odb := a.cdbSession()
	ctx := c.Request().Context()

	log.Info("called", logkey.NodeID, nodeId)

	// get node ID
	node, err := odb.NodeByNodeIDOrNodename(ctx, nodeId)
	if err != nil {
		log.Error("cannot find node", "node", nodeId, logkey.Error, err)
		return JSONProblemf(c, http.StatusNotFound, "node %s not found", nodeId)
	}

	// get attached rulesets with details
	groups := UserGroupsFromContext(c)
	isManager := IsManager(c)
	rulesets, err := odb.CompNodeAttachedRulesets(ctx, node.NodeID, groups, isManager)
	if err != nil {
		log.Error("cannot get attached rulesets", logkey.NodeID, node.NodeID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get attached rulesets for node %s", node.NodeID)
	}

	return c.JSON(http.StatusOK, rulesets)
}
