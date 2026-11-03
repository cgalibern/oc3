package cdb

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ObjectsUpdateLog handles, as ObjectUpdateLog does for one, the services_log
// and services_log_last changes of services set to avail, by a few statements
// for all of them: a statement by service, each committed alone, could not
// keep up with thousands of outdated services.
func (oDb *DB) ObjectsUpdateLog(ctx context.Context, svcIDs []string, avail string) error {
	defer logDuration("ObjectsUpdateLog", time.Now())
	if len(svcIDs) == 0 {
		return nil
	}
	last, err := oDb.ObjectStatusLogLastFromObjectIDs(ctx, svcIDs...)
	if err != nil {
		return fmt.Errorf("objectsUpdateLog can't get services_log_last: %w", err)
	}
	extend, transitions, set := objectStatusLogChanges(svcIDs, last, avail)
	// The interval of the previous avail is saved before services_log_last
	// is reset to the new one.
	if err := oDb.ObjectStatusLogUpdate(ctx, transitions...); err != nil {
		return fmt.Errorf("objectsUpdateLog can't save services_log changes: %w", err)
	}
	if err := oDb.ObjectStatusLogLastExtend(ctx, extend...); err != nil {
		return fmt.Errorf("objectsUpdateLog can't extend services_log_last: %w", err)
	}
	if err := oDb.ObjectStatusLogLastUpdate(ctx, set...); err != nil {
		return fmt.Errorf("objectsUpdateLog can't update services_log_last: %w", err)
	}
	oDb.SetChange("services_log")
	return nil
}

// objectStatusLogChanges returns, for the services set to avail, the ones whose
// current log is already avail, to extend; the current logs of another avail,
// to save as transitions; and the logs to set to avail, new or replacing a
// transition.
func objectStatusLogChanges(svcIDs []string, last []*DBObjectStatusLog, avail string) (extend []string, transitions, set []*DBObjectStatusLog) {
	bySvcID := make(map[string]*DBObjectStatusLog, len(last))
	for _, l := range last {
		bySvcID[l.SvcID] = l
	}
	for _, svcID := range svcIDs {
		prev, ok := bySvcID[svcID]
		switch {
		case ok && prev.AvailStatus == avail:
			extend = append(extend, svcID)
		case ok:
			transitions = append(transitions, prev)
			set = append(set, &DBObjectStatusLog{SvcID: svcID, AvailStatus: avail})
		default:
			set = append(set, &DBObjectStatusLog{SvcID: svcID, AvailStatus: avail})
		}
	}
	return
}

// ResourcesUpdateLog handles, as ResourceUpdateLog does for one, the resmon_log
// and resmon_log_last changes of resources set to status, by a few statements
// for all of them.
func (oDb *DB) ResourcesUpdateLog(ctx context.Context, resources []ResourceMeta, status string) error {
	defer logDuration("ResourcesUpdateLog", time.Now())
	if len(resources) == 0 {
		return nil
	}
	var nodeIDs, svcIDs []string
	seenNode := make(map[string]bool)
	seenSvc := make(map[string]bool)
	for _, r := range resources {
		if n := r.NID.String(); !seenNode[n] {
			seenNode[n] = true
			nodeIDs = append(nodeIDs, n)
		}
		if s := r.OID.String(); !seenSvc[s] {
			seenSvc[s] = true
			svcIDs = append(svcIDs, s)
		}
	}
	// The logs of the other resources of these nodes and services come along,
	// resourceStatusLogChanges ignores them.
	last, err := oDb.ResmonLogLastFromObjectIDs(ctx, nodeIDs, svcIDs...)
	if err != nil {
		return fmt.Errorf("resourcesUpdateLog can't get resmon_log_last: %w", err)
	}
	extend, transitions, set := resourceStatusLogChanges(resources, last, status)
	// The interval of the previous status is saved before resmon_log_last
	// is reset to the new one.
	if err := oDb.resourcesLogSaveTransitions(ctx, transitions); err != nil {
		return fmt.Errorf("resourcesUpdateLog can't save resmon_log changes: %w", err)
	}
	if err := oDb.resourcesLogLastExtend(ctx, extend); err != nil {
		return fmt.Errorf("resourcesUpdateLog can't extend resmon_log_last: %w", err)
	}
	if err := oDb.resourcesLogLastSet(ctx, set); err != nil {
		return fmt.Errorf("resourcesUpdateLog can't update resmon_log_last: %w", err)
	}
	oDb.SetChange("resmon_log")
	return nil
}

// The statements of ResourcesUpdateLog are those of ResourceUpdateLog for a
// batch: the ResmonLog* functions of the feed would also write res_log, and
// count other metrics.

// resourcesLogSaveTransitions saves in resmon_log the interval of the previous
// status of the resources, without res_log, as ResourceUpdateLog.
func (oDb *DB) resourcesLogSaveTransitions(ctx context.Context, l []*DBResmonLog) error {
	if len(l) == 0 {
		return nil
	}
	const valueList = "(?, ?, ?, ?, NOW(), ?)"
	query := "INSERT INTO `resmon_log` (`svc_id`, `node_id`, `rid`, `res_begin`, `res_end`, `res_status`) VALUES " +
		strings.Repeat(valueList+", ", len(l)-1) + valueList
	args := make([]any, 0, 5*len(l))
	for _, v := range l {
		args = append(args, v.SvcID, v.NodeID, v.RID, v.BeginAt, v.Status)
	}
	if count, err := oDb.execCountContext(ctx, query, args...); err != nil {
		return err
	} else if count > 0 {
		oDb.Metrics.ObjectStatusLogChange.Add(float64(count))
	}
	return nil
}

// resourcesLogLastExtend extends to now the current status logs of the
// resources, as ResourceUpdateLog.
func (oDb *DB) resourcesLogLastExtend(ctx context.Context, l []*DBInstanceResource) error {
	if len(l) == 0 {
		return nil
	}
	query := "UPDATE `resmon_log_last` SET `res_end` = NOW() WHERE (`svc_id`, `node_id`, `rid`) IN (" +
		strings.Repeat("(?,?,?),", len(l)-1) + "(?,?,?))"
	args := make([]any, 0, 3*len(l))
	for _, v := range l {
		args = append(args, v.SvcID, v.NodeID, v.RID)
	}
	if count, err := oDb.execCountContext(ctx, query, args...); err != nil {
		return err
	} else if count > 0 {
		oDb.Metrics.ObjectStatusLogExtend.Add(float64(count))
	}
	return nil
}

// resourcesLogLastSet starts the current status logs of the resources now, as
// ResourceUpdateLog: res_log is left as is, or to its default on insert.
func (oDb *DB) resourcesLogLastSet(ctx context.Context, l []*DBResmonLog) error {
	if len(l) == 0 {
		return nil
	}
	rows := make([][]any, len(l))
	for i, v := range l {
		rows[i] = []any{v.SvcID, v.NodeID, v.RID, v.Status}
	}
	given, args := givenRows([]string{"svc_id", "node_id", "rid", "res_status"}, rows)
	_, _, err := oDb.upsertRows(ctx, rowsUpsert{
		table:   "resmon_log_last",
		source:  "SELECT r.*, NOW() AS res_begin, NOW() AS res_end FROM (" + given + ") r",
		args:    args,
		columns: []string{"svc_id", "node_id", "rid", "res_status", "res_begin", "res_end"},
		match:   "t.node_id = s.node_id AND t.svc_id = s.svc_id AND t.rid = s.rid",
		update:  []string{"res_begin = s.res_begin", "res_end = s.res_end", "res_status = s.res_status"},
	})
	return err
}

// resourceStatusLogChanges returns, for the resources set to status, the ones
// whose current log is already of status, to extend; the current logs of
// another status, to save as transitions; and the logs to set to status, new or
// replacing a transition. As ResourceUpdateLog, it compares the statuses only.
func resourceStatusLogChanges(resources []ResourceMeta, last []*DBResmonLog, status string) (extend []*DBInstanceResource, transitions, set []*DBResmonLog) {
	byIDx := make(map[string]*DBResmonLog, len(last))
	for _, l := range last {
		byIDx[l.IDx()] = l
	}
	for _, r := range resources {
		svcID, nodeID := r.OID.String(), r.NID.String()
		prev, ok := byIDx[svcID+"@"+nodeID+":"+r.RID]
		switch {
		case ok && prev.Status == status:
			extend = append(extend, &DBInstanceResource{SvcID: svcID, NodeID: nodeID, RID: r.RID})
		case ok:
			transitions = append(transitions, prev)
			set = append(set, &DBResmonLog{SvcID: svcID, NodeID: nodeID, RID: r.RID, Status: status})
		default:
			set = append(set, &DBResmonLog{SvcID: svcID, NodeID: nodeID, RID: r.RID, Status: status})
		}
	}
	return
}
