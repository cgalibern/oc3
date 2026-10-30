package cdb

import (
	"database/sql"
	"maps"
	"slices"
	"testing"
)

func TestCompWithChildren(t *testing.T) {
	children := map[int][]int{1: {2, 3}, 2: {4}, 4: {1}}
	got := compWithChildren([]int{1}, children)
	slices.Sort(got)
	if !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("a loop ends: got %v", got)
	}
	if got := compWithChildren([]int{5}, children); !slices.Equal(got, []int{5}) {
		t.Errorf("no child: got %v", got)
	}
}

func TestCompRsetDepths(t *testing.T) {
	fset := func(i int64) sql.NullInt64 { return sql.NullInt64{Int64: i, Valid: true} }
	relations := map[int][]compRsetRelation{
		1: {{child: 2, rtype: "explicit"}, {child: 3, rtype: "contextual", fsetID: fset(10)}, {child: 4, rtype: "contextual", fsetID: fset(11)}},
		2: {{child: 5, rtype: "explicit"}, {child: 1, rtype: "explicit"}},
	}
	want := map[int]int{1: 0, 2: 1, 3: 1, 5: 2}
	if got := compRsetDepths(1, relations, map[int]bool{10: true}); !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCompAgentCleanLog(t *testing.T) {
	if got := compAgentCleanLog("a\n\t\x1b\x00b\xff😀"); got != "a\n\tb" {
		t.Errorf("got %q", got)
	}
}
