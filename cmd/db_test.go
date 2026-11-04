package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestDBKey(t *testing.T) {
	defer viper.Reset()
	viper.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
db:
  password: secret
  max_open_conns: 10
  conn_max_lifetime: 3m
scheduler:
  db:
    max_open_conns: 2
worker:
  db:
    conn_max_lifetime: 1m
worker.slow:
  db:
    max_open_conns: 30
`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		section, name, want string
	}{
		{sectionScheduler, "max_open_conns", "scheduler.db.max_open_conns"},
		{sectionScheduler, "conn_max_lifetime", "db.conn_max_lifetime"},
		{sectionFeeder, "max_open_conns", "db.max_open_conns"},
		{"worker.slow", "max_open_conns", "worker.slow.db.max_open_conns"},
		{"worker.slow", "conn_max_lifetime", "worker.db.conn_max_lifetime"},
		{"worker.fast", "max_open_conns", "db.max_open_conns"},
		{sectionDB, "max_open_conns", "db.max_open_conns"},
	}
	for _, c := range cases {
		if got := dbKey(c.section, c.name); got != c.want {
			t.Errorf("dbKey(%q, %q) = %q, want %q", c.section, c.name, got, c.want)
		}
	}
	if got := viper.GetInt(dbKey("worker.slow", "max_open_conns")); got != 30 {
		t.Errorf("worker.slow max_open_conns = %d, want 30", got)
	}
	if got := viper.GetDuration(dbKey("worker.slow", "conn_max_lifetime")).String(); got != "1m0s" {
		t.Errorf("worker.slow conn_max_lifetime = %s, want 1m0s", got)
	}
}
