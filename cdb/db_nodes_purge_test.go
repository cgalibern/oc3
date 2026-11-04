package cdb

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestDeleteNodesCascade checks the nodes are deleted from every table holding
// their rows, with one statement per table matching all the nodes.
func TestDeleteNodesCascade(t *testing.T) {
	rec := &deleteRecorder{}
	odb := &DB{DB: rec, HasTx: true, Metrics: testMetrics(), Session: NewSession(rec, nil)}
	if err := odb.DeleteNodesCascade(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, q := range rec.queries {
		if !strings.HasSuffix(q, "WHERE node_id IN (?,?)") {
			t.Errorf("query %q does not match the 2 nodes", q)
		}
		tables = append(tables, strings.Fields(q)[2])
	}
	if !slices.Equal(tables, nodeCascadeTables) {
		t.Errorf("tables %v, want %v", tables, nodeCascadeTables)
	}

	rec.queries = nil
	if err := odb.DeleteNodesCascade(context.Background(), []string{"a", ""}); err == nil {
		t.Error("empty node_id: no error")
	}
	if err := odb.DeleteNodesCascade(context.Background(), nil); err != nil {
		t.Errorf("no node: %s", err)
	}
	if len(rec.queries) != 0 {
		t.Errorf("queries %v, want none", rec.queries)
	}
}

// TestPurgeableNodeIDsDisabled checks no node is purgeable, and the database
// not queried, without a max age.
func TestPurgeableNodeIDsDisabled(t *testing.T) {
	odb := &DB{Metrics: testMetrics()}
	ids, err := odb.PurgeableNodeIDs(context.Background(), 0, 10)
	if err != nil || ids != nil {
		t.Errorf("ids %v, err %v", ids, err)
	}
}
