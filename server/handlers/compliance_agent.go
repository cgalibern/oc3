package serverhandlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cdb"
	"github.com/opensvc/oc3/server"
	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

type (
	// compAgentCaller is the node calling a /compliance/agent route, and the
	// object of this node the call is about, if any.
	compAgentCaller struct {
		nodeID string
		svcID  string
		slave  bool
	}

	// compAgentAttachment is what the attach and detach routes need of a
	// moduleset or a ruleset.
	compAgentAttachment struct {
		kind       string
		id         func(ctx context.Context, name string) (string, error)
		nodeList   func(ctx context.Context, nodeID string) ([]int, error)
		svcList    func(ctx context.Context, svcID string, slave bool) ([]int, error)
		attached   func(ctx context.Context, c compAgentCaller, id string) (bool, error)
		attachable func(ctx context.Context, c compAgentCaller, id string) (bool, error)
		attach     func(ctx context.Context, c compAgentCaller, id string) error
		detach     func(ctx context.Context, c compAgentCaller, id string) error
	}
)

func (a *Api) compAgentModuleset() compAgentAttachment {
	odb := a.ODB
	return compAgentAttachment{
		kind:     "moduleset",
		id:       odb.CompAgentModulesetID,
		nodeList: odb.CompNodeModulesets,
		svcList:  odb.CompServiceModulesets,
		attached: func(ctx context.Context, c compAgentCaller, id string) (bool, error) {
			if c.svcID != "" {
				return odb.CompModulesetSvcAttached(ctx, c.svcID, id, c.slave)
			}
			return odb.CompModulesetAttached(ctx, c.nodeID, id)
		},
		attachable: func(ctx context.Context, c compAgentCaller, id string) (bool, error) {
			if c.svcID != "" {
				return odb.CompModulesetSvcAttachable(ctx, c.svcID, id)
			}
			return odb.CompModulesetAttachable(ctx, c.nodeID, id)
		},
		attach: func(ctx context.Context, c compAgentCaller, id string) (err error) {
			if c.svcID != "" {
				_, err = odb.CompModulesetAttachService(ctx, c.svcID, id, c.slave)
			} else {
				_, err = odb.CompModulesetAttachNode(ctx, c.nodeID, id)
			}
			return err
		},
		detach: func(ctx context.Context, c compAgentCaller, id string) (err error) {
			if c.svcID != "" {
				_, err = odb.CompModulesetDetachService(ctx, c.svcID, id, c.slave)
			} else {
				_, err = odb.CompModulesetDetachNode(ctx, c.nodeID, []string{id})
			}
			return err
		},
	}
}

func (a *Api) compAgentRuleset() compAgentAttachment {
	odb := a.ODB
	return compAgentAttachment{
		kind:     "ruleset",
		id:       odb.CompAgentRulesetID,
		nodeList: odb.CompNodeRulesets,
		svcList:  odb.CompServiceRulesets,
		attached: func(ctx context.Context, c compAgentCaller, id string) (bool, error) {
			if c.svcID != "" {
				return odb.CompRulesetSvcAttached(ctx, c.svcID, id, c.slave)
			}
			return odb.CompRulesetAttached(ctx, c.nodeID, id)
		},
		attachable: func(ctx context.Context, c compAgentCaller, id string) (bool, error) {
			if c.svcID != "" {
				return odb.CompRulesetSvcAttachable(ctx, c.svcID, id)
			}
			return odb.CompRulesetAttachable(ctx, c.nodeID, id)
		},
		attach: func(ctx context.Context, c compAgentCaller, id string) (err error) {
			if c.svcID != "" {
				_, err = odb.CompRulesetAttachService(ctx, c.svcID, id, c.slave)
			} else {
				_, err = odb.CompRulesetAttachNode(ctx, c.nodeID, id)
			}
			return err
		},
		detach: func(ctx context.Context, c compAgentCaller, id string) (err error) {
			if c.svcID != "" {
				_, err = odb.CompRulesetDetachService(ctx, c.svcID, id, c.slave)
			} else {
				_, err = odb.CompRulesetDetachNode(ctx, c.nodeID, []string{id})
			}
			return err
		},
	}
}

// compAgent returns the calling node and its object at path, refusing the
// callers other than a node: these routes act on the node authenticated, and
// on nothing else.
func (a *Api) compAgent(c echo.Context, log *slog.Logger, ctx context.Context, path *string) (compAgentCaller, error) {
	var caller compAgentCaller
	if !IsAuthByNode(c) {
		return caller, httpErrorf(http.StatusForbidden, "node authentication required")
	}
	caller.nodeID, _ = c.Get(XNodeID).(string)
	if caller.nodeID == "" {
		return caller, httpErrorf(http.StatusForbidden, "node authentication required")
	}
	if path == nil || *path == "" {
		return caller, nil
	}
	node, err := a.ODB.NodeByNodeIDOrNodename(ctx, caller.nodeID)
	if err != nil {
		return caller, httpInternal(log, "cannot read the node", err)
	}
	if node == nil {
		return caller, httpErrorf(http.StatusNotFound, "node %s not found", caller.nodeID)
	}
	svc, err := a.ODB.ServiceByNameAndCluster(ctx, *path, node.ClusterID)
	if err != nil {
		return caller, httpInternal(log, "cannot read the object", err)
	}
	if svc == nil {
		return caller, httpErrorf(http.StatusNotFound, "object %s not found in the cluster of the node", *path)
	}
	caller.svcID = svc.SvcID
	if caller.slave, err = a.ODB.CompAgentNodeIsSlave(ctx, caller.nodeID, caller.svcID); err != nil {
		return caller, httpInternal(log, "cannot read the object instances", err)
	}
	return caller, nil
}

// GetComplianceAgentData handles GET /compliance/agent/data
func (a *Api) GetComplianceAgentData(c echo.Context, params server.GetComplianceAgentDataParams) error {
	log := echolog.GetLogHandler(c, "GetComplianceAgentData")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, params.Path)
	if err != nil {
		return httpProblem(c, err)
	}
	var modulesets []string
	if params.Moduleset != nil {
		modulesets = *params.Moduleset
	}
	data, err := a.ODB.CompAgentData(ctx, caller.nodeID, caller.svcID, caller.slave, modulesets)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the compliance data", err))
	}
	return c.JSON(http.StatusOK, data)
}

func compAgentPattern(p *string) string {
	if p == nil || *p == "" {
		return "%"
	}
	return *p
}

// GetComplianceAgentModulesets handles GET /compliance/agent/modulesets
func (a *Api) GetComplianceAgentModulesets(c echo.Context, params server.GetComplianceAgentModulesetsParams) error {
	log := echolog.GetLogHandler(c, "GetComplianceAgentModulesets")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, nil)
	if err != nil {
		return httpProblem(c, err)
	}
	l, err := a.ODB.CompAgentModulesets(ctx, caller.nodeID, compAgentPattern(params.Pattern))
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot list the modulesets", err))
	}
	return c.JSON(http.StatusOK, l)
}

// GetComplianceAgentRulesets handles GET /compliance/agent/rulesets
func (a *Api) GetComplianceAgentRulesets(c echo.Context, params server.GetComplianceAgentRulesetsParams) error {
	log := echolog.GetLogHandler(c, "GetComplianceAgentRulesets")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, nil)
	if err != nil {
		return httpProblem(c, err)
	}
	l, err := a.ODB.CompAgentRulesets(ctx, caller.nodeID, compAgentPattern(params.Pattern))
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot list the rulesets", err))
	}
	return c.JSON(http.StatusOK, l)
}

// PostComplianceAgentModuleset handles POST /compliance/agent/modulesets/{name}
func (a *Api) PostComplianceAgentModuleset(c echo.Context, name string, params server.PostComplianceAgentModulesetParams) error {
	return a.compAgentAttach(c, a.compAgentModuleset(), name, params.Path)
}

// DeleteComplianceAgentModuleset handles DELETE /compliance/agent/modulesets/{name}
func (a *Api) DeleteComplianceAgentModuleset(c echo.Context, name string, params server.DeleteComplianceAgentModulesetParams) error {
	return a.compAgentDetach(c, a.compAgentModuleset(), name, params.Path)
}

// PostComplianceAgentRuleset handles POST /compliance/agent/rulesets/{name}
func (a *Api) PostComplianceAgentRuleset(c echo.Context, name string, params server.PostComplianceAgentRulesetParams) error {
	return a.compAgentAttach(c, a.compAgentRuleset(), name, params.Path)
}

// DeleteComplianceAgentRuleset handles DELETE /compliance/agent/rulesets/{name}
func (a *Api) DeleteComplianceAgentRuleset(c echo.Context, name string, params server.DeleteComplianceAgentRulesetParams) error {
	return a.compAgentDetach(c, a.compAgentRuleset(), name, params.Path)
}

func (a *Api) compAgentAttach(c echo.Context, k compAgentAttachment, name string, path *string) error {
	log := echolog.GetLogHandler(c, "compAgentAttach")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, path)
	if err != nil {
		return httpProblem(c, err)
	}
	id, err := k.id(ctx, name)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the "+k.kind, err))
	}
	if id == "" {
		return httpProblem(c, httpErrorf(http.StatusNotFound, "%s %s does not exist", k.kind, name))
	}
	attached, err := k.attached(ctx, caller, id)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the attachment", err))
	}
	if attached {
		return c.JSON(http.StatusOK, map[string]string{"info": fmt.Sprintf("%s %s is already attached", k.kind, name)})
	}
	attachable, err := k.attachable(ctx, caller, id)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the publications", err))
	}
	if !attachable {
		return httpProblem(c, httpErrorf(http.StatusForbidden, "%s %s is not attachable", k.kind, name))
	}
	if err := k.attach(ctx, caller, id); err != nil {
		return httpProblem(c, httpInternal(log, "cannot attach the "+k.kind, err))
	}
	a.compAgentAttachLog(c, ctx, log, k.kind, id, caller, true)
	return c.JSON(http.StatusOK, map[string]string{"info": fmt.Sprintf("%s %s attached", k.kind, name)})
}

func (a *Api) compAgentAttachLog(c echo.Context, ctx context.Context, log *slog.Logger, kind, id string, caller compAgentCaller, attach bool) {
	if caller.svcID != "" {
		a.compAttachLog(c, ctx, log, kind, id, "", caller.svcID, attach)
		return
	}
	a.compAttachLog(c, ctx, log, kind, id, caller.nodeID, "", attach)
}

func (a *Api) compAgentDetach(c echo.Context, k compAgentAttachment, name string, path *string) error {
	log := echolog.GetLogHandler(c, "compAgentDetach")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, path)
	if err != nil {
		return httpProblem(c, err)
	}
	var ids []string
	if name == "all" {
		var l []int
		if caller.svcID != "" {
			l, err = k.svcList(ctx, caller.svcID, caller.slave)
		} else {
			l, err = k.nodeList(ctx, caller.nodeID)
		}
		if err != nil {
			return httpProblem(c, httpInternal(log, "cannot read the attachments", err))
		}
		for _, i := range l {
			ids = append(ids, fmt.Sprint(i))
		}
	} else {
		id, err := k.id(ctx, name)
		if err != nil {
			return httpProblem(c, httpInternal(log, "cannot read the "+k.kind, err))
		}
		if id == "" {
			return c.JSON(http.StatusOK, map[string]string{"info": fmt.Sprintf("%s %s does not exist", k.kind, name)})
		}
		attached, err := k.attached(ctx, caller, id)
		if err != nil {
			return httpProblem(c, httpInternal(log, "cannot read the attachment", err))
		}
		if !attached {
			return c.JSON(http.StatusOK, map[string]string{"info": fmt.Sprintf("%s %s is not attached", k.kind, name)})
		}
		ids = []string{id}
	}
	for _, id := range ids {
		if err := k.detach(ctx, caller, id); err != nil {
			return httpProblem(c, httpInternal(log, "cannot detach the "+k.kind, err))
		}
		a.compAgentAttachLog(c, ctx, log, k.kind, id, caller, false)
	}
	return c.JSON(http.StatusOK, map[string]string{"info": fmt.Sprintf("%d %s detached", len(ids), k.kind)})
}

// PostComplianceAgentLogs handles POST /compliance/agent/logs
func (a *Api) PostComplianceAgentLogs(c echo.Context, params server.PostComplianceAgentLogsParams) error {
	log := echolog.GetLogHandler(c, "PostComplianceAgentLogs")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, params.Path)
	if err != nil {
		return httpProblem(c, err)
	}
	var body server.PostComplianceAgentLogsJSONRequestBody
	if err := c.Bind(&body); err != nil {
		return httpProblem(c, httpErrorf(http.StatusBadRequest, "invalid request body: %s", err))
	}
	logs := make([]cdb.CompAgentLog, len(body))
	for i, l := range body {
		logs[i] = cdb.CompAgentLog{Module: l.Module, Status: l.Status, Log: l.Log, Action: l.Action}
		if l.RsetMd5 != nil {
			logs[i].RsetMD5 = *l.RsetMd5
		}
	}
	if err := a.ODB.CompAgentInsertLogs(ctx, caller.nodeID, caller.svcID, logs); err != nil {
		return httpProblem(c, httpInternal(log, "cannot record the logs", err))
	}
	for _, table := range []string{"comp_log", "comp_status"} {
		if err := a.ODB.Session.NotifyTableChangeWithData(ctx, table, map[string]any{"node_id": caller.nodeID}); err != nil {
			log.Warn("cannot notify the change", logkey.Error, err)
		}
	}
	return c.JSON(http.StatusAccepted, map[string]string{"info": fmt.Sprintf("%d logs recorded", len(logs))})
}
