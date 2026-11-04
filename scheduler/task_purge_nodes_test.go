package scheduler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestPurgeNodesMaxAge(t *testing.T) {
	const key = "scheduler.task.purge_nodes.max_age"
	defer viper.Set(key, nil)
	cases := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"30s", 0, true},
		{"soon", 0, true},
	}
	for _, c := range cases {
		viper.Set(key, c.value)
		got, err := purgeNodesMaxAge()
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", c.value, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %s, want %s", c.value, got, c.want)
		}
	}
}

// TestValidateMaxAgesPurgeNodesDisabled checks the scheduler starts with the
// default purge_nodes max_age, "0" (cmd/conf.go) disabling the purge, while
// the values the task rejects still fail.
func TestValidateMaxAgesPurgeNodesDisabled(t *testing.T) {
	const key = "scheduler.task.purge_nodes.max_age"
	defer viper.Set(key, nil)
	for value, wantErr := range map[string]bool{"0": false, "": false, "30d": false, "30s": true, "soon": true} {
		viper.Set(key, value)
		if err := ValidateMaxAges(); (err != nil) != wantErr {
			t.Errorf("purge_nodes max_age %q: err = %v, wantErr %v", value, err, wantErr)
		}
	}
}

// TestPurgeNodesByBatch checks the nodes are deleted by batches of the size,
// the files removed only for the nodes of the deleted batches, a failed batch
// or file removal reported without stopping the next ones, and no batch
// started once the context is done.
func TestPurgeNodesByBatch(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	var (
		batches [][]string
		removed []string
	)
	del := func(_ context.Context, batch []string) error {
		batches = append(batches, slices.Clone(batch))
		if batch[0] == "c" {
			return errors.New("lock wait timeout")
		}
		return nil
	}
	rm := func(id string) error {
		removed = append(removed, id)
		if id == "e" {
			return errors.New("permission denied")
		}
		return nil
	}
	purged, err := purgeNodesByBatch(context.Background(), ids, 2, del, rm)
	if want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}; !slices.EqualFunc(batches, want, slices.Equal[[]string]) {
		t.Errorf("batches %v, want %v", batches, want)
	}
	if want := []string{"a", "b", "e"}; !slices.Equal(removed, want) {
		t.Errorf("files removed for %v, want %v", removed, want)
	}
	if purged != 3 {
		t.Errorf("purged %d, want 3", purged)
	}
	if err == nil || !strings.Contains(err.Error(), "[c d]") || !strings.Contains(err.Error(), "node e files") {
		t.Errorf("err %v, want the failed batch and file removal", err)
	}

	batches, removed = nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	purged, err = purgeNodesByBatch(ctx, ids, 2, del, rm)
	if len(batches) != 0 || len(removed) != 0 || purged != 0 || !errors.Is(err, context.Canceled) {
		t.Errorf("done context: batches %v, removed %v, purged %d, err %v", batches, removed, purged, err)
	}
}
