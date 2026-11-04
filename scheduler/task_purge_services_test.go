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

func TestPurgeServicesMaxAge(t *testing.T) {
	const key = "scheduler.task.purge_services.max_age"
	defer viper.Set(key, nil)
	cases := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"3d", 3 * 24 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"30s", 0, true},
		{"soon", 0, true},
	}
	for _, c := range cases {
		viper.Set(key, c.value)
		got, err := purgeServicesMaxAge()
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", c.value, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %s, want %s", c.value, got, c.want)
		}
	}
}

// TestValidateMaxAgesPurgeServicesDisabled checks the scheduler starts with the
// default purge_services max_age, "0" (cmd/conf.go) disabling the age based
// purge, while the values the task rejects, and a max_age of 0 for the other
// tasks, still fail.
func TestValidateMaxAgesPurgeServicesDisabled(t *testing.T) {
	const key = "scheduler.task.purge_services.max_age"
	const other = "scheduler.task.scrub_object.max_age"
	defer viper.Set(key, nil)
	defer viper.Set(other, nil)
	viper.Set(other, "15m")
	for value, wantErr := range map[string]bool{"0": false, "": false, "2d": false, "30s": true, "soon": true} {
		viper.Set(key, value)
		if err := ValidateMaxAges(); (err != nil) != wantErr {
			t.Errorf("purge_services max_age %q: err = %v, wantErr %v", value, err, wantErr)
		}
	}
	viper.Set(key, "0")
	viper.Set(other, "0")
	if err := ValidateMaxAges(); err == nil {
		t.Errorf("scrub_object max_age %q: no error", "0")
	}
}

// TestDeleteServicesByBatch checks the services are deleted by batches of the
// size, a failed batch is not counted and does not stop the next ones, and no
// batch is started once the context is done.
func TestDeleteServicesByBatch(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	var batches [][]string
	del := func(_ context.Context, batch []string) error {
		batches = append(batches, slices.Clone(batch))
		if batch[0] == "c" {
			return errors.New("lock wait timeout")
		}
		return nil
	}
	purged, err := deleteServicesByBatch(context.Background(), &Task{}, ids, 2, del)
	if want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}; !slices.EqualFunc(batches, want, slices.Equal[[]string]) {
		t.Errorf("batches %v, want %v", batches, want)
	}
	if purged != 3 {
		t.Errorf("purged %d, want 3", purged)
	}
	if err == nil || !strings.Contains(err.Error(), "[c d]") {
		t.Errorf("err %v, want the failed batch", err)
	}

	batches = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	purged, err = deleteServicesByBatch(ctx, &Task{}, ids, 2, del)
	if len(batches) != 0 || purged != 0 || !errors.Is(err, context.Canceled) {
		t.Errorf("done context: batches %v, purged %d, err %v", batches, purged, err)
	}
}
