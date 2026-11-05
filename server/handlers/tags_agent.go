package serverhandlers

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/opensvc/oc3/cdb"
	"github.com/opensvc/oc3/server"
	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
)

// The /agent/tags routes serve the tag commands of the agents: a node lists
// the tags, creates one, and attaches or detaches a tag to itself, or to one
// of its objects with path. They act on the node authenticated and its
// objects only, as the /agent/compliance routes do: no user may call them,
// and a node has no other permission to check.

// GetAgentTags handles GET /agent/tags
func (a *Api) GetAgentTags(c echo.Context, params server.GetAgentTagsParams) error {
	log := echolog.GetLogHandler(c, "GetAgentTags")
	ctx := c.Request().Context()
	if _, err := a.compAgent(c, log, ctx, nil); err != nil {
		return httpProblem(c, err)
	}
	var pattern string
	if params.Pattern != nil {
		pattern = *params.Pattern
	}
	l, err := a.ODB.TagAgentNames(ctx, pattern)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot list the tags", err))
	}
	return c.JSON(http.StatusOK, l)
}

// PostAgentTags handles POST /agent/tags. A tag of the name existing with the
// same data and exclusion, or none given, is no error, as v2 answered.
func (a *Api) PostAgentTags(c echo.Context) error {
	log := echolog.GetLogHandler(c, "PostAgentTags")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, nil)
	if err != nil {
		return httpProblem(c, err)
	}
	var body server.PostAgentTagsJSONRequestBody
	if err := c.Bind(&body); err != nil {
		return httpProblem(c, httpErrorf(http.StatusBadRequest, "invalid request body: %s", err))
	}
	if body.Name == "" {
		return httpProblem(c, httpErrorf(http.StatusBadRequest, "a tag name is required"))
	}
	if body.Exclude != nil && *body.Exclude != "" {
		refusal, err := a.ODB.CheckTagExclude(ctx, *body.Exclude)
		if err != nil {
			return httpProblem(c, httpInternal(log, "cannot check the exclusion", err))
		}
		if refusal != nil {
			return httpProblem(c, httpErrorf(http.StatusBadRequest, "invalid exclusion %s: %s", *body.Exclude, refusal))
		}
	}
	existing, err := a.ODB.TagByName(ctx, body.Name)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the tag", err))
	}
	if existing != nil {
		switch {
		case body.Exclude != nil && *body.Exclude != existing.TagExclude:
			return httpProblem(c, httpErrorf(http.StatusConflict, "tag %s already exists with another exclusion", body.Name))
		case body.Data != nil && *body.Data != tagText(existing.TagData):
			return httpProblem(c, httpErrorf(http.StatusConflict, "tag %s already exists with other data", body.Name))
		}
		return c.JSON(http.StatusOK, server.TagAgentInfo{Info: fmt.Sprintf("tag %s already exists", body.Name)})
	}
	tag, err := a.ODB.InsertTag(ctx, body.Name, body.Exclude, body.Data)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot create the tag", err))
	}
	entry := cdb.LogEntry{
		Action: "tag.create",
		Fmt:    "tag '%(tag_name)s' created",
		Dict:   map[string]any{"tag_name": tag.TagName},
		Level:  "info",
	}
	if parsed, err := uuid.Parse(caller.nodeID); err == nil {
		entry.NodeID = &parsed
	}
	if err := a.ODB.Log(ctx, entry); err != nil {
		log.Error("cannot write audit log", logkey.Error, err)
	}
	a.notifyChanges(log, ctx)
	return c.JSON(http.StatusOK, server.TagAgentInfo{Info: fmt.Sprintf("tag %s created", tag.TagName)})
}

// tagText is the data of a tag as a text, its column read as any type.
func tagText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// GetAgentTagsAttached handles GET /agent/tags/attached
func (a *Api) GetAgentTagsAttached(c echo.Context, params server.GetAgentTagsAttachedParams) error {
	log := echolog.GetLogHandler(c, "GetAgentTagsAttached")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, params.Path)
	if err != nil {
		return httpProblem(c, err)
	}
	var l []cdb.TagAgentAttachment
	if caller.svcID != "" {
		l, err = a.ODB.TagAgentServiceAttachments(ctx, caller.svcID)
	} else {
		l, err = a.ODB.TagAgentNodeAttachments(ctx, caller.nodeID)
	}
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the tags attached", err))
	}
	resp := make([]server.TagAgentAttachment, len(l))
	for i, t := range l {
		resp[i] = server.TagAgentAttachment{TagName: t.TagName, TagData: t.TagData, TagAttachData: t.TagAttachData}
	}
	return c.JSON(http.StatusOK, resp)
}

// PostAgentTag handles POST /agent/tags/{name}
func (a *Api) PostAgentTag(c echo.Context, name string, params server.PostAgentTagParams) error {
	log := echolog.GetLogHandler(c, "PostAgentTag")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, params.Path)
	if err != nil {
		return httpProblem(c, err)
	}
	var body server.PostAgentTagJSONRequestBody
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&body); err != nil {
			return httpProblem(c, httpErrorf(http.StatusBadRequest, "invalid request body: %s", err))
		}
	}
	tag, err := a.ODB.TagByName(ctx, name)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the tag", err))
	}
	if tag == nil {
		return httpProblem(c, httpErrorf(http.StatusNotFound, "tag %s does not exist: create it first", name))
	}
	if caller.svcID != "" {
		return a.attachTagServiceChecked(c, log, ctx, tag, caller.svcID, body.AttachData)
	}
	return a.attachTagNodeChecked(c, log, ctx, tag, caller.nodeID, body.AttachData)
}

// DeleteAgentTag handles DELETE /agent/tags/{name}
func (a *Api) DeleteAgentTag(c echo.Context, name string, params server.DeleteAgentTagParams) error {
	log := echolog.GetLogHandler(c, "DeleteAgentTag")
	ctx := c.Request().Context()
	caller, err := a.compAgent(c, log, ctx, params.Path)
	if err != nil {
		return httpProblem(c, err)
	}
	tag, err := a.ODB.TagByName(ctx, name)
	if err != nil {
		return httpProblem(c, httpInternal(log, "cannot read the tag", err))
	}
	if tag == nil {
		return httpProblem(c, httpErrorf(http.StatusNotFound, "tag %s does not exist", name))
	}
	if caller.svcID != "" {
		return a.detachTagServiceChecked(c, log, ctx, tag, caller.svcID)
	}
	return a.detachTagNodeChecked(c, log, ctx, tag, caller.nodeID)
}
