package scheduler

import (
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
