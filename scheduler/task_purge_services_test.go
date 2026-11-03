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
