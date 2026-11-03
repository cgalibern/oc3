package scheduler

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// TestTasksHaveATimeout checks every task running a function has a timeout,
// the children included, which run under their own timeout.
func TestTasksHaveATimeout(t *testing.T) {
	for _, task := range Tasks {
		if task.fn != nil && task.timeout <= 0 {
			t.Errorf("%s: no timeout", task.name)
		}
		for _, child := range task.children {
			if child.timeout <= 0 {
				t.Errorf("%s: %s: no timeout", task.name, child.name)
			}
		}
	}
}

// TestExecChildrenOwnTimeout checks a child slow to the timeout of its parent
// does not leave the next children without time: each runs under its own.
func TestExecChildrenOwnTimeout(t *testing.T) {
	var deadlines []time.Duration
	child := func(ctx context.Context, _ *Task) error {
		d, _ := ctx.Deadline()
		deadlines = append(deadlines, time.Until(d))
		<-ctx.Done()
		return ctx.Err()
	}
	parent := Task{
		name:    "parent",
		timeout: 50 * time.Millisecond,
		children: TaskList{
			{name: "a", fn: child, timeout: 30 * time.Millisecond},
			{name: "b", fn: child, timeout: 30 * time.Millisecond},
		},
	}
	_ = parent.Exec(context.Background())
	if len(deadlines) != 2 {
		t.Fatalf("%d children run, want 2", len(deadlines))
	}
	for i, d := range deadlines {
		if d < 20*time.Millisecond {
			t.Errorf("child %d started with %s left, want its own 30ms", i, d)
		}
	}
}

// TestScrubByBatch checks the items are passed by batches, and none once the
// context is done or a batch failed.
func TestScrubByBatch(t *testing.T) {
	var batches [][]int
	fail := errors.New("lock wait timeout")
	fn := func(_ context.Context, batch []int) error {
		batches = append(batches, slices.Clone(batch))
		if batch[0] == 3 {
			return fail
		}
		return nil
	}
	if err := scrubByBatch(context.Background(), []int{1, 2, 3, 4, 5}, 2, fn); !errors.Is(err, fail) {
		t.Errorf("err %v, want the batch error", err)
	}
	if want := [][]int{{1, 2}, {3, 4}}; !slices.EqualFunc(batches, want, slices.Equal[[]int]) {
		t.Errorf("batches %v, want %v", batches, want)
	}

	batches = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scrubByBatch(ctx, []int{1, 2}, 2, fn); !errors.Is(err, context.Canceled) || len(batches) != 0 {
		t.Errorf("done context: err %v, batches %v", err, batches)
	}
}
