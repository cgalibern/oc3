package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScrapeQueues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`# HELP oc3_feed_queue_length Number of jobs waiting in a feed queue
# TYPE oc3_feed_queue_length gauge
oc3_feed_queue_length{queue="oc3:q:feed_daemon_ping"} 12
oc3_feed_queue_length{queue="oc3:q:feed_daemon_status"} 0
oc3_feed_jobs_running 3
`))
	}))
	defer srv.Close()
	lengths, err := metricsQueues(context.Background(), []string{srv.URL, "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if lengths["feed_daemon_ping"] != 12 || lengths["feed_daemon_status"] != 0 || len(lengths) != 2 {
		t.Fatalf("unexpected lengths: %v", lengths)
	}
	if _, err := metricsQueues(context.Background(), []string{"http://127.0.0.1:1"}); err == nil {
		t.Fatal("expected an error when no worker answers")
	}
}
