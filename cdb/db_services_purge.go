package cdb

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// PurgeableServiceIDs returns the svc_id of the services that no instance
// reports anymore (no svcmon row) and that are either tagged @purge, or,
// when maxAge > 0, whose status (svc_status_updated, falling back to the
// row's updated timestamp when the service never reported a status) is
// older than maxAge.
//
// The @purge tag is honored here whatever the service's age and whatever
// its cluster does: the feed only evaluates it when the service's own
// cluster sends a daemon status, which never happens again once the
// cluster is gone or reinstalled with a new cluster_id.
//
// At most limit ids are returned.
func (oDb *DB) PurgeableServiceIDs(ctx context.Context, maxAge time.Duration, limit int64) ([]string, error) {
	defer logDuration("PurgeableServiceIDs", time.Now())
	const query = "" +
		"SELECT `s`.`svc_id`" +
		" FROM `services` `s`" +
		" LEFT JOIN `svcmon` `m` ON `m`.`svc_id` = `s`.`svc_id`" +
		" LEFT JOIN (" +
		"   SELECT `st`.`svc_id` FROM `svc_tags` `st`" +
		"   JOIN `tags` `t` ON `t`.`tag_id` = `st`.`tag_id`" +
		"   WHERE `t`.`tag_name` = '@purge'" +
		" ) `p` ON `p`.`svc_id` = `s`.`svc_id`" +
		" WHERE `m`.`id` IS NULL" +
		"   AND (" +
		"     `p`.`svc_id` IS NOT NULL" +
		"     OR (? > 0 AND COALESCE(`s`.`svc_status_updated`, `s`.`updated`) < DATE_SUB(NOW(), INTERVAL ? SECOND))" +
		"   )" +
		" LIMIT ?"
	seconds := int64(maxAge.Seconds())
	rows, err := oDb.DB.QueryContext(ctx, query, seconds, seconds, limit)
	if err != nil {
		return nil, fmt.Errorf("PurgeableServiceIDs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("PurgeableServiceIDs: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("PurgeableServiceIDs: %w", err)
	}
	return ids, nil
}

// servicesLogPurgeBatch is the count of rows a purge of the services status
// logs deletes in one statement, each its own transaction.
const servicesLogPurgeBatch = 10000

// PurgeServicesLogOrphans deletes the availability logs, services_log and
// services_log_last, of the services that no longer exist and whose log ended
// before minAge: a log of a service being created, written meanwhile, is kept.
// The deletion of a service used to keep them, and the tables grew without end.
// It returns the number of deleted rows.
func (oDb *DB) PurgeServicesLogOrphans(ctx context.Context, minAge time.Duration) (int64, error) {
	defer logDuration("PurgeServicesLogOrphans", time.Now())
	var total int64
	for _, table := range []string{"services_log_last", "services_log"} {
		query := "DELETE FROM `" + table + "`" +
			" WHERE `svc_end` < DATE_SUB(NOW(), INTERVAL ? SECOND)" +
			" AND NOT EXISTS (SELECT 1 FROM `services` s WHERE s.`svc_id` = `" + table + "`.`svc_id`)" +
			" LIMIT " + strconv.Itoa(servicesLogPurgeBatch)
		var deleted int64
		for {
			count, err := oDb.execCountContext(ctx, query, maxAgeSeconds(minAge))
			if err != nil {
				return total + deleted, fmt.Errorf("purgeServicesLogOrphans %s: %w", table, err)
			}
			deleted += count
			if count < servicesLogPurgeBatch {
				break
			}
			if err := ctx.Err(); err != nil {
				return total + deleted, err
			}
		}
		if deleted > 0 {
			oDb.SetChange(table)
		}
		total += deleted
	}
	return total, nil
}

// PurgeOrphanServiceTags deletes the svc_tags rows whose service no longer
// exists, and returns the number of deleted rows.
func (oDb *DB) PurgeOrphanServiceTags(ctx context.Context) (int64, error) {
	defer logDuration("PurgeOrphanServiceTags", time.Now())
	const query = "" +
		"DELETE `st` FROM `svc_tags` `st`" +
		" LEFT JOIN `services` `s` ON `s`.`svc_id` = `st`.`svc_id`" +
		" WHERE `s`.`svc_id` IS NULL"
	count, err := oDb.execCountContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("PurgeOrphanServiceTags: %w", err)
	}
	if count > 0 {
		oDb.SetChange("svc_tags")
	}
	return count, nil
}
