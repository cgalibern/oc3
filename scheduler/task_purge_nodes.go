package scheduler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/viper"

	"github.com/opensvc/oc3/util/nodefiles"
)

// TaskPurgeNodes deletes, with all their related data and files, the nodes
// that report no instance anymore and that neither communicated nor changed
// for scheduler.task.purge_nodes.max_age: the rows of the node tables, the
// whisper series of their metrics (stats/nodes/<node_id>) and the git
// repository of their sysreport (sysreport/<node_id>).
//
// Disabled by default (max_age 0): production collectors keep the nodes
// gone until an operator deletes them. Set a max_age on collectors fed by
// short lived nodes, such as test labs, where the files of the nodes gone
// would fill the uploads directory.
//
// Each run handles at most scheduler.task.purge_nodes.batch_size nodes.
var TaskPurgeNodes = Task{
	name:    "purge_nodes",
	desc:    "delete the nodes without instance whose last communication and change are older than scheduler.task.purge_nodes.max_age (default 0: disabled), and their files",
	period:  time.Hour,
	fn:      taskPurgeNodes,
	timeout: time.Hour,
}

// purgeNodesMaxAge returns the configured max_age, 0 meaning the purge is
// disabled.
func purgeNodesMaxAge() (time.Duration, error) {
	const key = "scheduler.task.purge_nodes.max_age"
	switch viper.GetString(key) {
	case "", "0":
		return 0, nil
	}
	return parseMaxAgeKey(key)
}

// purgeNodesDeleteBatch is the count of nodes deleted together, by a
// statement per table, as purgeServicesDeleteBatch.
const purgeNodesDeleteBatch = 100

// purgeNodesByBatch deletes the nodes ids by batches of size with del, then
// the files of each node of a deleted batch with rm, and returns the count of
// nodes deleted. A batch failing is reported and the next ones are deleted,
// its files kept; none is started once ctx is done. A file not removed is
// reported, the node counted as deleted.
func purgeNodesByBatch(ctx context.Context, ids []string, size int, del func(context.Context, []string) error, rm func(string) error) (int, error) {
	var (
		purged int
		err    error
	)
	for batch := range slices.Chunk(ids, size) {
		if ctx.Err() != nil {
			return purged, errors.Join(err, ctx.Err())
		}
		if err1 := del(ctx, batch); err1 != nil {
			err = errors.Join(err, fmt.Errorf("purge nodes %v: %w", batch, err1))
			continue
		}
		purged += len(batch)
		for _, id := range batch {
			if err1 := rm(id); err1 != nil {
				err = errors.Join(err, fmt.Errorf("purge node %s files: %w", id, err1))
			}
		}
	}
	return purged, err
}

// removeNodeFiles removes the files of a node, serialized with the commits
// and purges of the sysreports.
func removeNodeFiles(nodeID string) error {
	uploads := viper.GetString("scheduler.directories.uploads")
	if uploads == "" {
		return nil
	}
	d := nodefiles.Dirs{Stats: filepath.Join(uploads, "stats"), Sysreport: filepath.Join(uploads, "sysreport")}
	sysreportMu.Lock()
	defer sysreportMu.Unlock()
	return d.Remove(nodeID)
}

func taskPurgeNodes(ctx context.Context, task *Task) error {
	age, err := purgeNodesMaxAge()
	if err != nil {
		return err
	}
	if age == 0 {
		return nil
	}
	odb := task.DB()
	batchSize := viper.GetInt64("scheduler.task.purge_nodes.batch_size")
	if batchSize <= 0 {
		batchSize = 1000
	}
	ids, err := odb.PurgeableNodeIDs(ctx, age, batchSize)
	if err != nil {
		return err
	}
	purged, err := purgeNodesByBatch(ctx, ids, purgeNodesDeleteBatch, odb.DeleteNodesCascade, removeNodeFiles)
	task.Infof("max_age: %s. batch size: %d. candidates: %d. purged nodes: %d", age, batchSize, len(ids), purged)
	if err1 := odb.Session.NotifyChanges(ctx); err1 != nil {
		err = errors.Join(err, err1)
	}
	return err
}
