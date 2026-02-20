package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"

	"github.com/opensvc/oc3/scheduler"
	"github.com/opensvc/oc3/xauth"
)

type (
	schedulerT struct {
		db      *sql.DB
		redis   *redis.Client
		section string
	}
)

func (t *schedulerT) authMiddleware(publicPath, publicPrefix []string) echo.MiddlewareFunc {
	return AuthMiddleware(union.New(
		xauth.NewPublicStrategy(publicPath, publicPrefix),
	))
}

func newScheduler() (*schedulerT, error) {
	if err := setup(sectionScheduler); err != nil {
		return nil, err
	}
	if db, err := newDatabase(); err != nil {
		return nil, err
	} else {
		t := &schedulerT{db: db, section: sectionScheduler}
		return t, nil
	}
}

func scheduleExec(name string) error {
	t, err := newScheduler()
	if err != nil {
		return err
	}
	task := scheduler.NewTask(name, t.db, newEv())
	if task.IsZero() {
		return fmt.Errorf("task not found")
	}
	return task.Exec(context.Background())
}

func scheduleList() error {
	scheduler.Tasks.Print()
	return nil
}

func startScheduler() error {
	t, err := newScheduler()
	if err != nil {
		return err
	}
	if ok, errC := start(t); ok {
		slog.Info(fmt.Sprintf("%s started", t.Section()))
		go func() {
			if err := <-errC; err != nil {
				slog.Error(fmt.Sprintf("%s stopped: %s", t.Section(), err))
			}
		}()
	}

	sched := &scheduler.Scheduler{
		DB: t.db,
		Ev: newEv(),
	}
	return sched.Run()
}

func (t *schedulerT) Section() string {
	return t.section
}
