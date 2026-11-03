package cdb

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestObjectStatusLogChanges checks a service whose log is already of the avail
// is extended, one of another avail saved as a transition and set, and one
// without log set.
func TestObjectStatusLogChanges(t *testing.T) {
	begin := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	up := &DBObjectStatusLog{SvcID: "up", AvailStatus: "up", BeginAt: begin}
	last := []*DBObjectStatusLog{
		{SvcID: "undef", AvailStatus: "undef"},
		up,
		{SvcID: "other", AvailStatus: "up"},
	}
	extend, transitions, set := objectStatusLogChanges([]string{"undef", "up", "new"}, last, "undef")
	if want := []string{"undef"}; !slices.Equal(extend, want) {
		t.Errorf("extend %v, want %v", extend, want)
	}
	if len(transitions) != 1 || transitions[0] != up {
		t.Errorf("transitions %v, want the up log, with its begin", transitions)
	}
	var setIDs []string
	for _, l := range set {
		if l.AvailStatus != "undef" {
			t.Errorf("%s set to %q, want undef", l.SvcID, l.AvailStatus)
		}
		setIDs = append(setIDs, l.SvcID)
	}
	if want := []string{"up", "new"}; !slices.Equal(setIDs, want) {
		t.Errorf("set %v, want %v", setIDs, want)
	}
}

// TestResourceStatusLogChanges checks the resources are told by service, node
// and rid, the logs of the other resources read along being ignored.
func TestResourceStatusLogChanges(t *testing.T) {
	svc, node, otherNode := uuid.New(), uuid.New(), uuid.New()
	fs := &DBResmonLog{SvcID: svc.String(), NodeID: node.String(), RID: "fs#1", Status: "up"}
	last := []*DBResmonLog{
		{SvcID: svc.String(), NodeID: node.String(), RID: "ip#0", Status: "undef"},
		fs,
		{SvcID: svc.String(), NodeID: otherNode.String(), RID: "fs#1", Status: "undef"},
	}
	resources := []ResourceMeta{
		{OID: svc, NID: node, RID: "ip#0"},
		{OID: svc, NID: node, RID: "fs#1"},
		{OID: svc, NID: node, RID: "app#0"},
	}
	extend, transitions, set := resourceStatusLogChanges(resources, last, "undef")
	if len(extend) != 1 || extend[0].IDx() != svc.String()+"@"+node.String()+":ip#0" {
		t.Errorf("extend %v, want ip#0", extend)
	}
	if len(transitions) != 1 || transitions[0] != fs {
		t.Errorf("transitions %v, want the fs#1 log", transitions)
	}
	var setRIDs []string
	for _, l := range set {
		if l.Status != "undef" || l.NodeID != node.String() {
			t.Errorf("%s set to %q on %s, want undef on %s", l.RID, l.Status, l.NodeID, node)
		}
		setRIDs = append(setRIDs, l.RID)
	}
	if want := []string{"fs#1", "app#0"}; !slices.Equal(setRIDs, want) {
		t.Errorf("set %v, want %v", setRIDs, want)
	}
}

// affectedResult is a sql.Result of a count of affected rows.
type affectedResult int64

func (r affectedResult) LastInsertId() (int64, error) { return 0, nil }
func (r affectedResult) RowsAffected() (int64, error) { return int64(r), nil }

// stmtRecorder is a DBOperater recording the statements executed, each
// affecting one row.
type stmtRecorder struct {
	DBOperater
	queries []string
}

func (r *stmtRecorder) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	r.queries = append(r.queries, query)
	return affectedResult(1), nil
}

// TestResourcesLogStatementsKeepResLog checks the statements of
// ResourcesUpdateLog are those of ResourceUpdateLog: res_log is not written,
// and the same metrics are counted.
func TestResourcesLogStatementsKeepResLog(t *testing.T) {
	rec := &stmtRecorder{}
	m := newMetrics()
	odb := &DB{DB: rec, HasTx: true, Metrics: m, Session: NewSession(rec, nil)}
	ctx := context.Background()
	l := []*DBResmonLog{{SvcID: "s", NodeID: "n", RID: "fs#1", Status: "up", Log: "mounted"}}
	if err := odb.resourcesLogSaveTransitions(ctx, l); err != nil {
		t.Fatal(err)
	}
	if err := odb.resourcesLogLastExtend(ctx, []*DBInstanceResource{{SvcID: "s", NodeID: "n", RID: "ip#0"}}); err != nil {
		t.Fatal(err)
	}
	if err := odb.resourcesLogLastSet(ctx, []*DBResmonLog{{SvcID: "s", NodeID: "n", RID: "fs#1", Status: "undef"}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.queries) == 0 {
		t.Fatal("no statement")
	}
	for _, q := range rec.queries {
		if strings.Contains(q, "res_log`") || strings.Contains(q, "res_log ") || strings.Contains(q, "res_log,") || strings.Contains(q, ".res_log") {
			t.Errorf("res_log written by %q", q)
		}
	}
	if got := counterValue(m.ObjectStatusLogChange); got != 1 {
		t.Errorf("ObjectStatusLogChange %v, want 1", got)
	}
	if got := counterValue(m.ObjectStatusLogExtend); got != 1 {
		t.Errorf("ObjectStatusLogExtend %v, want 1", got)
	}
	if got := counterValue(m.ResourceStatusLogChange) + counterValue(m.ResourceStatusLogExtend); got != 0 {
		t.Errorf("ResourceStatusLog* counted %v, want 0 as ResourceUpdateLog", got)
	}
}

// counterValue returns the value of the counter c.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}
