package serverhandlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetNodeComplianceCandidateRulesets handles GET /nodes/{node_id}/compliance/candidate_rulesets
func (a *Api) GetNodeComplianceCandidateRulesets(c echo.Context, nodeId string) error {
	log := echolog.GetLogHandler(c, "GetNodeComplianceCandidateRulesets")
	odb := a.cdbSession()
	ctx := c.Request().Context()

	log.Info("called", logkey.NodeID, nodeId)

	// get node ID
	node, err := a.cdbSession().NodeByNodeIDOrNodename(c.Request().Context(), nodeId)
	if err != nil {
		log.Error("cannot find node", "node", nodeId, logkey.Error, err)
		return JSONProblemf(c, http.StatusNotFound, "node %s not found", nodeId)
	}

	// get rulesets already attached to the node
	attachedRulesets, err := odb.CompNodeRulesets(ctx, node.NodeID)
	if err != nil {
		log.Error("cannot get attached rulesets", logkey.NodeID, node.NodeID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get attached rulesets for node %s", node.NodeID)
	}

	// get candidate rulesets
	groups := UserGroupsFromContext(c)
	isManager := IsManager(c)
	candidates, err := odb.CompNodeCandidateRulesets(ctx, node.NodeID, attachedRulesets, groups, isManager)
	if err != nil {
		log.Error("cannot get candidate rulesets", logkey.NodeID, node.NodeID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get candidate rulesets for node %s", node.NodeID)
	}

	return c.JSON(http.StatusOK, candidates)

}
