package cdb

import (
	"context"
	"fmt"
	"time"
)

// PurgeableNodeIDs returns the node_id of the nodes that report no instance
// anymore (no svcmon row) and whose last communication (last_comm, set by the
// daemon pings and statuses) and last change of their row (updated, set by
// the system feeds and the API) are both older than maxAge, or absent.
//
// At most limit ids are returned. A maxAge <= 0 returns none.
func (oDb *DB) PurgeableNodeIDs(ctx context.Context, maxAge time.Duration, limit int64) ([]string, error) {
	defer logDuration("PurgeableNodeIDs", time.Now())
	if maxAge <= 0 {
		return nil, nil
	}
	const query = "" +
		"SELECT `n`.`node_id`" +
		" FROM `nodes` `n`" +
		" WHERE `n`.`node_id` IS NOT NULL AND `n`.`node_id` <> ''" +
		"   AND NOT EXISTS (SELECT 1 FROM `svcmon` `m` WHERE `m`.`node_id` = `n`.`node_id`)" +
		"   AND (`n`.`last_comm` IS NULL OR `n`.`last_comm` < DATE_SUB(NOW(), INTERVAL ? SECOND))" +
		"   AND (`n`.`updated` IS NULL OR `n`.`updated` < DATE_SUB(NOW(), INTERVAL ? SECOND))" +
		" LIMIT ?"
	seconds := int64(maxAge.Seconds())
	rows, err := oDb.DB.QueryContext(ctx, query, seconds, seconds, limit)
	if err != nil {
		return nil, fmt.Errorf("PurgeableNodeIDs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("PurgeableNodeIDs: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PurgeableNodeIDs: %w", err)
	}
	return ids, nil
}
