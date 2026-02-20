package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"

	"github.com/opensvc/oc3/runner"
	"github.com/opensvc/oc3/xauth"
)

type (
	runnerT struct {
		db      *sql.DB
		section string
	}
)

func (t *runnerT) Section() string { return t.section }

func (t *runnerT) authMiddleware(publicPath, publicPrefix []string) echo.MiddlewareFunc {
	return AuthMiddleware(union.New(
		xauth.NewPublicStrategy(publicPath, publicPrefix),
	))
}

func newRunner() (*runnerT, error) {
	if err := setup(sectionRunner); err != nil {
		return nil, err
	}
	if db, err := newDatabase(); err != nil {
		return nil, err
	} else {
		t := &runnerT{db: db, section: sectionRunner}
		return t, nil
	}
}

func startRunner() error {
	t, err := newRunner()
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

	ad := &runner.ActionDaemon{
		DB:  t.db,
		Ev:  newEv(),
		Ctx: context.Background(),
	}

	return ad.Run()
}
