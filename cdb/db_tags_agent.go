package cdb

import (
	"context"
	"fmt"
	"strings"
)

// TagAgentNames returns the names of the tags matching the sql LIKE pattern,
// lowercased and sorted, as the agents list them. A pattern not starting or
// not ending with % is completed, so a text matches the names containing it,
// as v2 did.
func (oDb *DB) TagAgentNames(ctx context.Context, pattern string) ([]string, error) {
	query := "SELECT LOWER(tag_name) FROM tags WHERE id > 0"
	var args []any
	if pattern != "" {
		if !strings.HasPrefix(pattern, "%") {
			pattern = "%" + pattern
		}
		if !strings.HasSuffix(pattern, "%") {
			pattern += "%"
		}
		query += " AND tag_name LIKE ?"
		args = append(args, pattern)
	}
	query += " ORDER BY tag_name"
	rows, err := oDb.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("TagAgentNames: %w", err)
	}
	defer func() { _ = rows.Close() }()
	l := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("TagAgentNames: %w", err)
		}
		l = append(l, name)
	}
	return l, rows.Err()
}

// TagAgentAttachment is a tag attached to a node or a service, with the data
// of the tag and of the attachment, as the agents show them.
type TagAgentAttachment struct {
	TagName       string
	TagData       string
	TagAttachData string
}

// TagAgentNodeAttachments returns the tags attached to the node, sorted by name.
func (oDb *DB) TagAgentNodeAttachments(ctx context.Context, nodeID string) ([]TagAgentAttachment, error) {
	return oDb.tagAgentAttachments(ctx, "node_tags", "node_id", nodeID)
}

// TagAgentServiceAttachments returns the tags attached to the service, sorted
// by name.
func (oDb *DB) TagAgentServiceAttachments(ctx context.Context, svcID string) ([]TagAgentAttachment, error) {
	return oDb.tagAgentAttachments(ctx, "svc_tags", "svc_id", svcID)
}

func (oDb *DB) tagAgentAttachments(ctx context.Context, table, column, id string) ([]TagAgentAttachment, error) {
	query := fmt.Sprintf(`SELECT LOWER(t.tag_name), COALESCE(t.tag_data, ''), COALESCE(a.tag_attach_data, '')
		FROM %s a JOIN tags t ON t.tag_id = a.tag_id
		WHERE a.%s = ?
		ORDER BY t.tag_name`, table, column)
	rows, err := oDb.DB.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("tagAgentAttachments %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	l := make([]TagAgentAttachment, 0)
	for rows.Next() {
		var t TagAgentAttachment
		if err := rows.Scan(&t.TagName, &t.TagData, &t.TagAttachData); err != nil {
			return nil, fmt.Errorf("tagAgentAttachments %s: %w", table, err)
		}
		l = append(l, t)
	}
	return l, rows.Err()
}
