package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

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
var TaskPurgeServices = Task{
	name:    "purge_services",
	desc:    "delete the services without instance that are tagged @purge or whose status is older than scheduler.task.purge_services.max_age (default 0: disabled)",
	period:  time.Hour,
	fn:      taskPurgeServices,
	timeout: time.Hour,
}

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
	var purged int
	for _, id := range ids {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
			break
		}
		if err1 := odb.DeleteServiceCascade(ctx, id); err1 != nil {
			err = errors.Join(err, fmt.Errorf("purge service %s: %w", id, err1))
			continue
		}
		purged++
	}
	orphanTags, err1 := odb.PurgeOrphanServiceTags(ctx)
	if err1 != nil {
		err = errors.Join(err, err1)
	}
	task.Infof("max_age: %s. batch size: %d. candidates: %d. purged services: %d. orphan tags deleted: %d", age, batchSize, len(ids), purged, orphanTags)
	if err1 := odb.Session.NotifyChanges(ctx); err1 != nil {
		err = errors.Join(err, err1)
	}
	return err
}
