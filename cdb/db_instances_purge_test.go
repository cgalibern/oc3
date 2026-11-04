package cdb

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
)

// purgeRecorder is a DBOperater recording the statements executed, failing
// those on the table fail, and affecting no row on the table empty.
type purgeRecorder struct {
	DBOperater
	queries []string
	args    [][]any
	fail    string
	empty   string
}

func (r *purgeRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.queries = append(r.queries, query)
	r.args = append(r.args, args)
	switch {
	case strings.HasPrefix(query, "DELETE FROM "+r.fail+" "):
		return nil, context.DeadlineExceeded
	case strings.HasPrefix(query, "DELETE FROM "+r.empty+" "):
		return affectedResult(0), nil
	}
	return affectedResult(1), nil
}

// TestPurgeInstances checks the instances are deleted from each instance table
// by one statement, as PurgeInstance for one; a table failing is reported, the
// next ones still purged; only the tables with deleted rows are changed.
func TestPurgeInstances(t *testing.T) {
	rec := &purgeRecorder{fail: "resmon", empty: "saves"}
	odb := &DB{DB: rec, HasTx: true, Metrics: testMetrics(), Session: NewSession(rec, nil)}
	ids := []InstanceID{{svcID: "s1", nodeID: "n1"}, {svcID: "s2", nodeID: "n2"}}

	err := odb.PurgeInstances(context.Background(), ids)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "delete from resmon") {
		t.Errorf("got %v, want the resmon deletion error", err)
	}
	if len(rec.queries) != len(instanceTables) {
		t.Fatalf("%d statements, want one by table: %d", len(rec.queries), len(instanceTables))
	}
	for i, table := range instanceTables {
		want := "DELETE FROM " + table + " WHERE (`svc_id`, `node_id`) IN ((?,?),(?,?))"
		if rec.queries[i] != want {
			t.Errorf("statement %d\n%q\nwant\n%q", i, rec.queries[i], want)
		}
		if want := []any{"s1", "n1", "s2", "n2"}; !slices.Equal(rec.args[i], want) {
			t.Errorf("%s args %v, want %v", table, rec.args[i], want)
		}
	}
	var changed []string
	for table := range odb.Session.tables {
		changed = append(changed, table)
	}
	slices.Sort(changed)
	var want []string
	for _, table := range instanceTables {
		if table != "resmon" && table != "saves" {
			want = append(want, table)
		}
	}
	slices.Sort(want)
	if !slices.Equal(changed, want) {
		t.Errorf("changed %v, want %v", changed, want)
	}
}

// TestPurgeInstancesNone checks no statement is executed without instance.
func TestPurgeInstancesNone(t *testing.T) {
	rec := &purgeRecorder{}
	odb := &DB{DB: rec, HasTx: true, Metrics: testMetrics(), Session: NewSession(rec, nil)}
	if err := odb.PurgeInstances(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.queries) != 0 {
		t.Errorf("%d statements, want none", len(rec.queries))
	}
}
