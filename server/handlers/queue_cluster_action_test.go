package serverhandlers

import (
	"encoding/json"
	"testing"
)

func TestClusterActionCommand(t *testing.T) {
	for action, want := range map[string]string{
		"freeze": "freeze --node *",
		"thaw":   "unfreeze --node *",
		"abort":  "abort --node *",
		// The rolling upgrade, run as `om cluster update pkg` by the agent.
		"updatepkg": "cluster update pkg",
	} {
		if got := clusterActionCommand(action); got != want {
			t.Errorf("%s: got %q, want %q", action, got, want)
		}
	}
	for _, action := range []string{"start", "stop", "drain", "reboot", "join", "leave"} {
		if _, ok := clusterActionWords[action]; ok {
			t.Errorf("%s is accepted on a cluster", action)
		}
	}
}

func TestClusterActionEntry(t *testing.T) {
	var e actionEntry
	if err := json.Unmarshal([]byte(`{"cluster_id":"c1","action":"freeze","rid":"fs#1"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.ClusterID != "c1" {
		t.Fatalf("cluster_id: %q", e.ClusterID)
	}
	// A cluster entry with a rid is not merged with instance entries.
	out := factorizeActionEntries([]actionEntry{e, {SvcID: "s", NodeID: "n", Action: "freeze", Rid: "fs#2"}})
	if len(out) != 2 || out[0].ClusterID != "c1" {
		t.Errorf("factorized: %+v", out)
	}
}
