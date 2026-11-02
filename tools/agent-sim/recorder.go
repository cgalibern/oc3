package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

type (
	// recorder keeps the requests of the run: every request in samples.csv,
	// the windows of the report in windows.csv, the totals in summary.json.
	recorder struct {
		mu      sync.Mutex
		begin   time.Time
		window  map[string]*endpointStats
		total   map[string]*endpointStats
		samples *bufio.Writer
		file    *os.File
		windows *os.File
		queues  map[string]int64
		qmu     sync.Mutex
		out     io.Writer
	}

	endpointStats struct {
		Count   int         `json:"count"`
		Errors  int         `json:"errors"`
		Bytes   int64       `json:"bytes"`
		Codes   map[int]int `json:"codes"`
		LastErr string      `json:"last_error,omitempty"`
		ms      []float64
	}

	endpointSummary struct {
		Endpoint string      `json:"endpoint"`
		Count    int         `json:"count"`
		Errors   int         `json:"errors"`
		Rate     float64     `json:"rate_per_s"`
		KBAvg    float64     `json:"kb_avg"`
		P50      float64     `json:"p50_ms"`
		P95      float64     `json:"p95_ms"`
		P99      float64     `json:"p99_ms"`
		Max      float64     `json:"max_ms"`
		Codes    map[int]int `json:"codes"`
		LastErr  string      `json:"last_error,omitempty"`
	}
)

func newRecorder(dir string, out io.Writer) (*recorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "samples.csv"))
	if err != nil {
		return nil, err
	}
	w, err := os.Create(filepath.Join(dir, "windows.csv"))
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintln(w, "elapsed_s,requests,rate_per_s,errors,p50_ms,p95_ms,p99_ms,max_ms,queued")
	r := &recorder{
		begin:   time.Now(),
		window:  map[string]*endpointStats{},
		total:   map[string]*endpointStats{},
		samples: bufio.NewWriterSize(f, 1<<20),
		file:    f,
		windows: w,
		queues:  map[string]int64{},
		out:     out,
	}
	_, _ = fmt.Fprintln(r.samples, "elapsed_ms,endpoint,code,duration_ms,bytes,error")
	return r, nil
}

func (r *recorder) add(endpoint string, code int, d time.Duration, size int, err error) {
	ms := float64(d.Microseconds()) / 1000
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range []map[string]*endpointStats{r.window, r.total} {
		st, ok := m[endpoint]
		if !ok {
			st = &endpointStats{Codes: map[int]int{}}
			m[endpoint] = st
		}
		st.Count++
		st.Bytes += int64(size)
		st.Codes[code]++
		st.ms = append(st.ms, ms)
		if err != nil {
			st.Errors++
			st.LastErr = err.Error()
		}
	}
	errS := ""
	if err != nil {
		errS = strings.NewReplacer(",", ";", "\n", " ").Replace(err.Error())
		if len(errS) > 120 {
			errS = errS[:120]
		}
	}
	_, _ = fmt.Fprintf(r.samples, "%d,%s,%d,%.1f,%d,%s\n", time.Since(r.begin).Milliseconds(), endpoint, code, ms, size, errS)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*p))]
}

// report prints and records the window just ended, and starts a new one.
func (r *recorder) report(period time.Duration) {
	r.mu.Lock()
	window := r.window
	r.window = map[string]*endpointStats{}
	r.mu.Unlock()
	var all []float64
	count, errs := 0, 0
	for _, st := range window {
		all = append(all, st.ms...)
		count += st.Count
		errs += st.Errors
	}
	slices.Sort(all)
	queued, detail := r.queueTotal()
	elapsed := time.Since(r.begin).Seconds()
	_, _ = fmt.Fprintf(r.windows, "%.0f,%d,%.1f,%d,%.1f,%.1f,%.1f,%.1f,%d\n", elapsed, count, float64(count)/period.Seconds(), errs,
		percentile(all, .5), percentile(all, .95), percentile(all, .99), percentile(all, 1), queued)
	_, _ = fmt.Fprintf(r.out, "%6.0fs %7.1f req/s  err %-5d p50 %7.1fms  p95 %7.1fms  p99 %7.1fms  max %8.1fms  queued %d%s\n",
		elapsed, float64(count)/period.Seconds(), errs, percentile(all, .5), percentile(all, .95), percentile(all, .99), percentile(all, 1), queued, detail)
}

// sampleQueues follows the length of the worker queues of oc3 in redis: a
// queue that grows is a worker that does not keep up with the feed.
func (r *recorder) sampleQueues(ctx context.Context, addr string, dir string) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer func() { _ = client.Close() }()
	f, err := os.Create(filepath.Join(dir, "queues.csv"))
	if err != nil {
		_, _ = fmt.Fprintln(r.out, "queues:", err)
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintln(f, "elapsed_s,queue,length")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		keys, err := client.Keys(ctx, "oc3:q:*").Result()
		if err == nil {
			lengths := map[string]int64{}
			for _, k := range keys {
				if n, err := client.LLen(ctx, k).Result(); err == nil {
					lengths[strings.TrimPrefix(k, "oc3:q:")] = n
					_, _ = fmt.Fprintf(f, "%.0f,%s,%d\n", time.Since(r.begin).Seconds(), strings.TrimPrefix(k, "oc3:q:"), n)
				}
			}
			r.qmu.Lock()
			r.queues = lengths
			r.qmu.Unlock()
		} else if ctx.Err() == nil {
			_, _ = fmt.Fprintln(r.out, "queues:", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *recorder) queueTotal() (int64, string) {
	r.qmu.Lock()
	defer r.qmu.Unlock()
	var total int64
	var l []string
	for k, n := range r.queues {
		total += n
		if n > 0 {
			l = append(l, fmt.Sprintf("%s=%d", k, n))
		}
	}
	sort.Strings(l)
	if len(l) == 0 {
		return total, ""
	}
	return total, " (" + strings.Join(l, " ") + ")"
}

func (r *recorder) close(dir string) ([]endpointSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.samples.Flush()
	_ = r.file.Close()
	_ = r.windows.Close()
	elapsed := time.Since(r.begin).Seconds()
	var l []endpointSummary
	for endpoint, st := range r.total {
		slices.Sort(st.ms)
		l = append(l, endpointSummary{
			Endpoint: endpoint, Count: st.Count, Errors: st.Errors, Rate: float64(st.Count) / elapsed,
			KBAvg: float64(st.Bytes) / float64(st.Count) / 1024,
			P50:   percentile(st.ms, .5), P95: percentile(st.ms, .95), P99: percentile(st.ms, .99), Max: percentile(st.ms, 1),
			Codes: st.Codes, LastErr: st.LastErr,
		})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Count > l[j].Count })
	b, _ := json.MarshalIndent(l, "", "  ")
	return l, os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644)
}

func printSummary(w io.Writer, l []endpointSummary) {
	_, _ = fmt.Fprintf(w, "\n%-32s %8s %6s %8s %8s %9s %9s %9s %9s\n", "endpoint", "count", "errors", "req/s", "KB avg", "p50 ms", "p95 ms", "p99 ms", "max ms")
	for _, e := range l {
		_, _ = fmt.Fprintf(w, "%-32s %8d %6d %8.2f %8.1f %9.1f %9.1f %9.1f %9.1f\n", e.Endpoint, e.Count, e.Errors, e.Rate, e.KBAvg, e.P50, e.P95, e.P99, e.Max)
	}
	for _, e := range l {
		if e.LastErr != "" {
			_, _ = fmt.Fprintf(w, "last error of %s: %s\n", e.Endpoint, e.LastErr)
		}
	}
}
