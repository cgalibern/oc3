package cdb

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
)

// execRecorder is a DBOperater recording the statements executed.
type execRecorder struct {
	DBOperater
	queries []string
	args    [][]any
	err     error
}

func (r *execRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.queries = append(r.queries, query)
	r.args = append(r.args, args)
	return nil, r.err
}

// TestServicesAvailabilityUpdate checks the rates are given in the order of the
// svc_id, a nil rate or trend passed as NULL, and only update existing services.
func TestServicesAvailabilityUpdate(t *testing.T) {
	rate, trend := 99.5, -0.25
	query, args := servicesAvailabilityUpdate(map[string]ServiceAvailability{
		"s2": {},
		"s1": {Rate: &rate, Trend: &trend},
		"s3": {Rate: &rate},
	})
	want := "UPDATE services s JOIN (" +
		"SELECT ? AS svc_id, ? AS svc_availability, ? AS svc_availability_trend UNION ALL SELECT ?, ?, ? UNION ALL SELECT ?, ?, ?" +
		") r ON s.svc_id = r.svc_id" +
		" SET s.svc_availability = r.svc_availability, s.svc_availability_trend = r.svc_availability_trend, s.svc_availability_updated = NOW()"
	if query != want {
		t.Errorf("query\n%q\nwant\n%q", query, want)
	}
	if want := []any{"s1", 99.5, -0.25, "s2", nil, nil, "s3", 99.5, nil}; !slices.Equal(args, want) {
		t.Errorf("args %v, want %v", args, want)
	}
}

// TestSetServicesAvailabilityOneStatement checks the rates of a batch of
// services are stored by a single statement, and none when there is no rate.
func TestSetServicesAvailabilityOneStatement(t *testing.T) {
	rec := &execRecorder{}
	odb := &DB{DB: rec}
	rate := 100.0
	rates := make(map[string]ServiceAvailability)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		rates[id] = ServiceAvailability{Rate: &rate}
	}
	if err := odb.SetServicesAvailability(context.Background(), rates); err != nil {
		t.Fatal(err)
	}
	if len(rec.queries) != 1 {
		t.Fatalf("%d statements, want 1", len(rec.queries))
	}
	if len(rec.args[0]) != 3*len(rates) {
		t.Errorf("%d args, want %d", len(rec.args[0]), 3*len(rates))
	}
	if err := odb.SetServicesAvailability(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.queries) != 1 {
		t.Errorf("a statement executed without rate")
	}
}

// TestSetServicesAvailabilityError checks the error of the statement is returned.
func TestSetServicesAvailabilityError(t *testing.T) {
	rec := &execRecorder{err: context.DeadlineExceeded}
	odb := &DB{DB: rec}
	err := odb.SetServicesAvailability(context.Background(), map[string]ServiceAvailability{"a": {}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want a context.DeadlineExceeded", err)
	}
}
