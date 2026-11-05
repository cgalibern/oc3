package serverhandlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cdb"
	"github.com/opensvc/oc3/util/logkey"
)

// resolveTagByRecordID looks up a tag by its record id (tags.id). On failure the
// problem response is written and errRequestDenied returned, so that the caller
// stops instead of going on with a nil tag.
func (a *Api) resolveTagByRecordID(c echo.Context, log *slog.Logger, ctx context.Context, id int) (*cdb.Tag, error) {
	tag, err := a.ODB.TagByID(ctx, id)
	if err != nil {
		log.Error("cannot get tag", logkey.TagID, id, logkey.Error, err)
		return nil, denyRequest(c, http.StatusInternalServerError, "cannot get tag")
	}
	if tag == nil {
		return nil, denyRequest(c, http.StatusNotFound, "tag %d not found", id)
	}
	return tag, nil
}

// resolveTagByKey looks up a tag by its tag_id, or else by its record id when the
// key is a number. On failure the problem response is written and
// errRequestDenied returned.
func (a *Api) resolveTagByKey(c echo.Context, log *slog.Logger, ctx context.Context, key string) (*cdb.Tag, error) {
	tag, err := a.ODB.TagByTagID(ctx, key)
	if err != nil {
		log.Error("cannot get tag", logkey.TagID, key, logkey.Error, err)
		return nil, denyRequest(c, http.StatusInternalServerError, "cannot get tag")
	}
	if tag != nil {
		return tag, nil
	}
	if id, convErr := strconv.Atoi(key); convErr == nil {
		return a.resolveTagByRecordID(c, log, ctx, id)
	}
	return nil, denyRequest(c, http.StatusNotFound, "tag %s not found", key)
}

func (a *Api) attachTagNode(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, nodeId string, tagAttachData *string) error {
	odb := a.ODB

	if nodeId == "" {
		return JSONProblemf(c, http.StatusBadRequest, "invalid node_id: ''")
	}

	responsible, err := odb.NodeResponsible(ctx, nodeId, UserGroupsFromContext(c), IsManager(c))
	if err != nil {
		log.Error("cannot check if user is responsible for the node", logkey.NodeID, nodeId, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check if user is responsible for node %s", nodeId)
	}
	if !responsible {
		log.Info("user is not responsible for this node", logkey.NodeID, nodeId)
		return JSONProblemf(c, http.StatusForbidden, "user is not responsible for node %s", nodeId)
	}
	return a.attachTagNodeChecked(c, log, ctx, tag, nodeId, tagAttachData)
}

// attachTagNodeChecked attaches tag to the node, or updates the attach data,
// the caller allowed to: a user responsible for the node, or the node itself.
func (a *Api) attachTagNodeChecked(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, nodeId string, tagAttachData *string) error {
	odb := a.ODB

	current, err := odb.GetNodeTagAttachment(ctx, nodeId, tag.TagID)
	if err != nil {
		log.Error("cannot get node tag attachment", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get node tag attachment")
	}

	allowed, err := odb.NodeTagAttachAllowed(ctx, nodeId, tag.TagName)
	if err != nil {
		log.Error("cannot check tag compatibility", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check tag compatibility")
	}
	if !allowed {
		return JSONProblemf(c, http.StatusConflict, "tag '%s' is not compatible with other tags attached to node '%s'", tag.TagName, nodeId)
	}

	var info, logFmt string
	switch {
	case current != nil:
		if tagAttachData != nil && *tagAttachData != current.TagAttachData {
			if err := odb.UpdateNodeTagAttachData(ctx, nodeId, tag.TagID, *tagAttachData); err != nil {
				log.Error("cannot update node tag attach data", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
				return JSONProblemf(c, http.StatusInternalServerError, "cannot update node tag attach data")
			}
			info = fmt.Sprintf("node '%s' tag '%s' attach data updated", nodeId, tag.TagName)
			logFmt = "node '%(node_id)s' tag '%(tag_name)s' attach data updated"
		} else {
			return c.JSON(http.StatusOK, map[string]string{
				"info": fmt.Sprintf("tag '%s' already attached to node '%s'", tag.TagName, nodeId),
			})
		}
	default:
		if err := odb.AttachNodeTag(ctx, nodeId, tag.TagID); err != nil {
			log.Error("cannot attach tag to node", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
			return JSONProblemf(c, http.StatusInternalServerError, "cannot attach tag to node")
		}
		// The data given with a first attachment is kept with it: a form-based tag
		// is attached with its form filled in one request.
		if tagAttachData != nil && *tagAttachData != "" {
			if err := odb.UpdateNodeTagAttachData(ctx, nodeId, tag.TagID, *tagAttachData); err != nil {
				log.Error("cannot set node tag attach data", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
				return JSONProblemf(c, http.StatusInternalServerError, "cannot set node tag attach data")
			}
		}
		info = fmt.Sprintf("node '%s' tag '%s' attached", nodeId, tag.TagName)
		logFmt = "node '%(node_id)s' tag '%(tag_name)s' attached"
	}

	a.logNodeTagChange(c, log, ctx, nodeId, logFmt, map[string]any{"node_id": nodeId, "tag_name": tag.TagName})
	a.notifyChanges(log, ctx)

	return c.JSON(http.StatusOK, map[string]string{"info": info})
}

// detachTagNode detaches tag from the node.
func (a *Api) detachTagNode(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, nodeId string) error {
	odb := a.ODB

	responsible, err := odb.NodeResponsible(ctx, nodeId, UserGroupsFromContext(c), IsManager(c))
	if err != nil {
		log.Error("cannot check if user is responsible for the node", logkey.NodeID, nodeId, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check if user is responsible for node %s", nodeId)
	}
	if !responsible {
		log.Info("user is not responsible for this node", logkey.NodeID, nodeId)
		return JSONProblemf(c, http.StatusForbidden, "user is not responsible for node %s", nodeId)
	}
	return a.detachTagNodeChecked(c, log, ctx, tag, nodeId)
}

// detachTagNodeChecked detaches tag from the node, the caller allowed to.
func (a *Api) detachTagNodeChecked(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, nodeId string) error {
	odb := a.ODB

	n, err := odb.DetachNodeTag(ctx, nodeId, tag.TagID)
	if err != nil {
		log.Error("cannot detach tag from node", logkey.NodeID, nodeId, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot detach tag from node")
	}
	if n == 0 {
		return c.JSON(http.StatusOK, map[string]string{"info": "tag already detached"})
	}

	a.logNodeTagChange(c, log, ctx, nodeId, "tag '%(tag_name)s' detached", map[string]any{"tag_name": tag.TagName})
	a.notifyChanges(log, ctx)

	return c.JSON(http.StatusOK, map[string]string{"info": "tag detached"})
}

// attachTagService attaches tag to the service, or updates the attach data
func (a *Api) attachTagService(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, svcId string, tagAttachData *string) error {
	odb := a.ODB

	svcRow, err := a.resolveServiceRow(c, log, ctx, svcId)
	if err != nil {
		return err
	}
	svc := svcRow.SvcID

	responsible, err := odb.ServiceResponsible(ctx, svc, UserGroupsFromContext(c), IsManager(c))
	if err != nil {
		log.Error("cannot check if user is responsible for the service", "svc_id", svc, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check if user is responsible for service %s", svc)
	}
	if !responsible {
		log.Info("user is not responsible for this service", "svc_id", svc)
		return JSONProblemf(c, http.StatusForbidden, "user is not responsible for service %s", svc)
	}
	return a.attachTagServiceChecked(c, log, ctx, tag, svc, tagAttachData)
}

// attachTagServiceChecked attaches tag to the service of id svc, or updates
// the attach data, the caller allowed to: a user responsible for the service,
// or a node of its cluster.
func (a *Api) attachTagServiceChecked(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, svc string, tagAttachData *string) error {
	odb := a.ODB

	current, err := odb.GetServiceTagAttachment(ctx, svc, tag.TagID)
	if err != nil {
		log.Error("cannot get service tag attachment", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot get service tag attachment")
	}

	allowed, err := odb.ServiceTagAttachAllowed(ctx, svc, tag.TagName)
	if err != nil {
		log.Error("cannot check tag compatibility", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check tag compatibility")
	}
	if !allowed {
		return JSONProblemf(c, http.StatusConflict, "tag '%s' is not compatible with other tags attached to service '%s'", tag.TagName, svc)
	}

	var info, logFmt string
	switch {
	case current != nil:
		if tagAttachData != nil && *tagAttachData != current.TagAttachData {
			if err := odb.UpdateServiceTagAttachData(ctx, svc, tag.TagID, *tagAttachData); err != nil {
				log.Error("cannot update service tag attach data", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
				return JSONProblemf(c, http.StatusInternalServerError, "cannot update service tag attach data")
			}
			info = fmt.Sprintf("service '%s' tag '%s' attach data updated", svc, tag.TagName)
			logFmt = "service '%(svc_id)s' tag '%(tag_name)s' attach data updated"
		} else {
			return c.JSON(http.StatusOK, map[string]string{
				"info": fmt.Sprintf("tag '%s' already attached to service '%s'", tag.TagName, svc),
			})
		}
	default:
		if err := odb.AttachServiceTag(ctx, svc, tag.TagID); err != nil {
			log.Error("cannot attach tag to service", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
			return JSONProblemf(c, http.StatusInternalServerError, "cannot attach tag to service")
		}
		// The data given with a first attachment is kept with it: a form-based tag
		// is attached with its form filled in one request.
		if tagAttachData != nil && *tagAttachData != "" {
			if err := odb.UpdateServiceTagAttachData(ctx, svc, tag.TagID, *tagAttachData); err != nil {
				log.Error("cannot set service tag attach data", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
				return JSONProblemf(c, http.StatusInternalServerError, "cannot set service tag attach data")
			}
		}
		info = fmt.Sprintf("service '%s' tag '%s' attached", svc, tag.TagName)
		logFmt = "service '%(svc_id)s' tag '%(tag_name)s' attached"
	}

	a.logServiceTagChange(c, log, ctx, svc, logFmt, map[string]any{"svc_id": svc, "tag_name": tag.TagName})
	a.notifyChanges(log, ctx)

	return c.JSON(http.StatusOK, map[string]string{"info": info})
}

// detachTagService detaches tag from the service.
func (a *Api) detachTagService(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, svcId string) error {
	odb := a.ODB

	svcRow, err := a.resolveServiceRow(c, log, ctx, svcId)
	if err != nil {
		return err
	}
	svc := svcRow.SvcID

	responsible, err := odb.ServiceResponsible(ctx, svc, UserGroupsFromContext(c), IsManager(c))
	if err != nil {
		log.Error("cannot check if user is responsible for the service", "svc_id", svc, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot check if user is responsible for service %s", svc)
	}
	if !responsible {
		log.Info("user is not responsible for this service", "svc_id", svc)
		return JSONProblemf(c, http.StatusForbidden, "user is not responsible for service %s", svc)
	}
	return a.detachTagServiceChecked(c, log, ctx, tag, svc)
}

// detachTagServiceChecked detaches tag from the service of id svc, the
// caller allowed to.
func (a *Api) detachTagServiceChecked(c echo.Context, log *slog.Logger, ctx context.Context, tag *cdb.Tag, svc string) error {
	odb := a.ODB

	n, err := odb.DetachServiceTag(ctx, svc, tag.TagID)
	if err != nil {
		log.Error("cannot detach tag from service", "svc_id", svc, logkey.TagID, tag.TagID, logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot detach tag from service")
	}
	if n == 0 {
		return c.JSON(http.StatusOK, map[string]string{"info": "tag already detached"})
	}

	a.logServiceTagChange(c, log, ctx, svc, "tag '%(tag_name)s' detached", map[string]any{"tag_name": tag.TagName})
	a.notifyChanges(log, ctx)

	return c.JSON(http.StatusOK, map[string]string{"info": "tag detached"})
}

func (a *Api) logNodeTagChange(c echo.Context, log *slog.Logger, ctx context.Context, nodeId, logFmt string, dict map[string]any) {
	userEmail, _ := c.Get(XUserEmail).(string)
	logEntry := cdb.LogEntry{
		Action: "node.tag",
		User:   userEmail,
		Fmt:    logFmt,
		Dict:   dict,
		Level:  "info",
	}
	if parsed, err := uuid.Parse(nodeId); err == nil {
		logEntry.NodeID = &parsed
	}
	if logErr := a.ODB.Log(ctx, logEntry); logErr != nil {
		log.Error("cannot write audit log", logkey.Error, logErr)
	}
}

func (a *Api) logServiceTagChange(c echo.Context, log *slog.Logger, ctx context.Context, svcId, logFmt string, dict map[string]any) {
	userEmail, _ := c.Get(XUserEmail).(string)
	logEntry := cdb.LogEntry{
		Action: "service.tag",
		User:   userEmail,
		Fmt:    logFmt,
		Dict:   dict,
		Level:  "info",
	}
	if parsed, err := uuid.Parse(svcId); err == nil {
		logEntry.SvcID = &parsed
	}
	if logErr := a.ODB.Log(ctx, logEntry); logErr != nil {
		log.Error("cannot write audit log", logkey.Error, logErr)
	}
}

func (a *Api) notifyChanges(log *slog.Logger, ctx context.Context) {
	if err := a.ODB.Session.NotifyChanges(ctx); err != nil {
		log.Error("cannot notify changes", logkey.Error, err)
	}
}
