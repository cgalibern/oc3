package scheduler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/viper"
)

const (
	// purgeServicesDeleteBatch is the count of services deleted together, by a
	// statement per table: each statement commits alone, so a service at a time
	// cost a commit per table and service, too slow to purge the candidates within
	// the period. A bigger batch would make long deletions on the large tables.
	purgeServicesDeleteBatch = 100

	// purgeServicesLogOrphansMinAge is the age of the end of the status log of a
	// service gone, after which the log is purged: a service being created may
	// have its log written before its services row.
	purgeServicesLogOrphansMinAge = time.Hour
)

var (
	// TaskPurgeServices deletes, with all their related data, the services that
	// no instance reports anymore and that are either tagged @purge or, when
	// scheduler.task.purge_services.max_age is set, whose status is older than
	// that age. It also drops the svc_tags rows left behind by deleted services.
	//
	// The age based purge is disabled by default (max_age 0): production
	// collectors usually keep the services that left their cluster until an
	// operator deletes them. Set a short max_age on collectors fed by short
	// lived clusters, such as test labs, to keep the services, dashboard and
	// history tables small.
	//
	// Each run handles at most scheduler.task.purge_services.batch_size
	// services, so a large backlog is drained over several runs without
	// holding the database for too long.
	TaskPurgeServices = Task{
		name:    "purge_services",
		desc:    "delete the services without instance that are tagged @purge or whose status is older than scheduler.task.purge_services.max_age (default 0: disabled)",
		period:  time.Hour,
		fn:      taskPurgeServices,
		timeout: time.Hour,
	}
)

// purgeServicesMaxAge returns the configured max_age, 0 meaning the age
// based purge is disabled.
func purgeServicesMaxAge() (time.Duration, error) {
	const key = "scheduler.task.purge_services.max_age"
	switch viper.GetString(key) {
	case "", "0":
		return 0, nil
	}
	return parseMaxAgeKey(key)
}

// deleteServicesByBatch deletes the services ids by batches of size with del,
// and returns the count of services deleted. A batch failing is reported and
// the next ones are deleted; none is started once ctx is done.
func deleteServicesByBatch(ctx context.Context, task *Task, ids []string, size int, del func(context.Context, []string) error) (int, error) {
	var (
		purged int
		err    error
	)
	for batch := range slices.Chunk(ids, size) {
		if ctx.Err() != nil {
			return purged, errors.Join(err, ctx.Err())
		}
		task.Infof("deleteServicesByBatch services %d", len(ids))
		if err1 := del(ctx, batch); err1 != nil {
			err = errors.Join(err, fmt.Errorf("purge services %v: %w", batch, err1))
			continue
		}
		purged += len(batch)
	}
	return purged, err
}

func taskPurgeServices(ctx context.Context, task *Task) error {
	odb := task.DB()
	age, err := purgeServicesMaxAge()
	if err != nil {
		return err
	}
	batchSize := viper.GetInt64("scheduler.task.purge_services.batch_size")
	if batchSize <= 0 {
		batchSize = 1000
	}
	ids, err := odb.PurgeableServiceIDs(ctx, age, batchSize)
	if err != nil {
		return err
	}
	task.Infof("purgeable services %d", len(ids))
	purged, err := deleteServicesByBatch(ctx, task, ids, purgeServicesDeleteBatch, odb.DeleteServicesCascade)
	orphanTags, err1 := odb.PurgeOrphanServiceTags(ctx)
	if err1 != nil {
		err = errors.Join(err, err1)
	}
	orphanLogs, err1 := odb.PurgeServicesLogOrphans(ctx, purgeServicesLogOrphansMinAge)
	if err1 != nil {
		err = errors.Join(err, err1)
	}
	task.Infof("max_age: %s. batch size: %d. candidates: %d. purged services: %d. orphan tags deleted: %d. orphan status logs deleted: %d", age, batchSize, len(ids), purged, orphanTags, orphanLogs)
	if err1 := odb.Session.NotifyChanges(ctx); err1 != nil {
		err = errors.Join(err, err1)
	}
	return err
}
