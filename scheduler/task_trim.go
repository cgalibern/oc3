package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

var TaskTrim = Task{
	name:    "trim",
	period:  time.Minute,
	fn:      taskTrimRun,
	timeout: 2 * time.Hour,
}

// deleteBatched executes the deletion query in batches until no rows are affected.
func deleteBatched(ctx context.Context, task *Task, table, dateCol, where string) error {
	batchSize := getBatchSize(table)
	retention := getRetentionDays(table)
	odb := task.DB()

	totalDeleted, batchCount, err := odb.DeleteBatched(ctx, table, dateCol, batchSize, retention, where)
	if err != nil {
		return err
	}

	task.Infof("%s: deletion complete. retention: %d days. batch size: %d. total batches: %d. total rows deleted: %d", table, retention, batchSize, batchCount, totalDeleted)
	return nil
}

func getBatchSize(table string) int64 {
	n := viper.GetInt64(fmt.Sprintf("scheduler.task.trim.table.%s.batch_size", table))
	if n == 0 {
		n = viper.GetInt64("scheduler.task.trim.batch_size")
	}
	return n
}

func getRetentionDays(table string) int {
	days := viper.GetInt(fmt.Sprintf("scheduler.task.trim.table.%s.retention", table))
	if days == 0 {
		days = viper.GetInt("scheduler.task.trim.retention")
	}
	return days
}

func taskTrimRun(ctx context.Context, task *Task) (err error) {
	err = errors.Join(err, deleteBatched(ctx, task, "saves", "save_date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "log", "log_date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "stat_day_disk_array", "day", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "stat_day_disk_array_dg", "day", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "stat_day_disk_app", "day", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "stat_day_disk_app_dg", "day", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "switches", "sw_updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "comp_log", "run_date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "comp_log_daily", "run_date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "resmon_log", "res_end", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "svcmon_log", "mon_end", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "svcactions", "begin", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "dashboard_events", "dash_end", "AND NOT `dash_end` IS NULL"))
	err = errors.Join(err, deleteBatched(ctx, task, "packages", "pkg_updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "patches", "patch_updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "node_ip", "updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "node_users", "updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "node_groups", "updated", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "comp_run_ruleset", "date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "links", "link_last_consultation_date", ""))
	err = errors.Join(err, deleteBatched(ctx, task, "services_log", "svc_end", ""))
	return
}
