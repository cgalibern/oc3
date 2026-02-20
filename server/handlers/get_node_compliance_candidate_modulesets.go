package serverhandlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// GetNodeComplianceCandidateModulesets handles GET /nodes/{node_id}/compliance/candidate_modulesets
func (a *Api) GetNodeComplianceCandidateModulesets(c echo.Context, nodeId string) error {
	log := echolog.GetLogHandler(c, "GetNodeComplianceCandidateModulesets")
	odb := a.cdbSession()
	ctx := c.Request().Context()

	log.Info("called", logkey.NodeID, nodeId)

	// get node ID
	node, err := odb.NodeByNodeIDOrNodename(ctx, nodeId)
	if err != nil {
		log.Error("cannot find node", "node", nodeId, logkey.Error, err)
		return JSONProblemf(c, http.StatusNotFound, "node %s not found", nodeId)
	}

	// get modulesets already attached to the node
	attachedModulesets, err := odb.CompNodeModulesets(ctx, node.NodeID)
	if err != nil {
		log.Error("cannot get attached modulesets", logkey.NodeID, node.NodeID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get attached modulesets for node %s", node.NodeID)
	}

	// get candidate modulesets
	groups := UserGroupsFromContext(c)
	isManager := IsManager(c)
	candidates, err := odb.CompNodeCandidateModulesets(ctx, node.NodeID, attachedModulesets, groups, isManager)
	if err != nil {
		log.Error("cannot get candidate modulesets", logkey.NodeID, node.NodeID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get candidate modulesets for node %s", node.NodeID)
	}

	return c.JSON(http.StatusOK, candidates)
}
