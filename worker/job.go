package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

type (
	// BaseJob is a base struct to compose jobs.
	BaseJob struct {
		ctx   context.Context
		redis *redis.Client
		db    DBOperater
		oDb   *opensvcDB
		ev    EventPublisher

		name   string
		detail string
		now    time.Time
	}

	operation struct {
		desc string
		do   func() error

		continueOnError bool
		skipOp          func() bool
	}

	LogResulter interface {
		LogResult()
	}

	Operationer interface {
		Operations() []operation
	}

	DBGetter interface {
		DB() DBOperater
	}
)

func RunJob(j JobRunner) error {
	name := j.Name()
	detail := j.Detail()
	defer logDurationInfo(fmt.Sprintf("%s %s", name, detail), time.Now())
	slog.Info(fmt.Sprintf("%s starting %s", name, detail))

	ops := j.Operations()

	err := runOps(ops...)
	if err != nil {
		if tx, ok := j.DB().(DBTxer); ok {
			slog.Debug(fmt.Sprintf("%s rollbacking on error %s", name, detail))
			if err := tx.Rollback(); err != nil {
				slog.Error(fmt.Sprintf("%s rollback on error failed %s: %s", name, detail, err))
			}
		}
		return err
	} else if tx, ok := j.DB().(DBTxer); ok {
		slog.Debug(fmt.Sprintf("%s commiting %s", name, detail))
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}
	if r, ok := j.(LogResulter); ok {
		r.LogResult()
	}
	slog.Info(fmt.Sprintf("%s done %s", name, detail))
	return nil
}

func (j *BaseJob) PrepareDB(ctx context.Context, db *sql.DB, withTx bool) error {
	switch withTx {
	case true:
		if tx, err := db.BeginTx(ctx, nil); err != nil {
			return err
		} else {
			j.db = tx
			j.oDb = &opensvcDB{db: tx, tChanges: make(map[string]struct{})}
		}
	case false:
		j.db = db
		j.oDb = &opensvcDB{db: db, tChanges: make(map[string]struct{})}
	}
	j.ctx = ctx
	return nil
}

func (j *BaseJob) DB() DBOperater {
	return j.db
}

func (j *BaseJob) SetRedis(r *redis.Client) {
	j.redis = r
}

func (j *BaseJob) SetEv(ev EventPublisher) {
	j.ev = ev
}

func (j *BaseJob) Name() string {
	return j.name
}

func (j *BaseJob) Detail() string {
	return j.detail
}

func (j *BaseJob) dbNow() (err error) {
	rows, err := j.db.QueryContext(j.ctx, "SELECT NOW()")
	if err != nil {
		return err
	}
	if rows == nil {
		return fmt.Errorf("no result rows for SELECT NOW()")
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("no result rows next for SELECT NOW()")
	}
	if err := rows.Scan(&j.now); err != nil {
		return err
	}
	return nil
}

func runOps(ops ...operation) error {
	for _, op := range ops {
		if op.skipOp != nil && op.skipOp() {
			continue
		}
		begin := time.Now()
		err := op.do()
		duration := time.Now().Sub(begin)
		if err != nil {
			operationDuration.
				With(prometheus.Labels{"desc": op.desc, "status": operationStatusFailed}).
				Observe(duration.Seconds())
			if op.continueOnError {
				continue
			}
			return err
		}
		operationDuration.
			With(prometheus.Labels{"desc": op.desc, "status": operationStatusOk}).
			Observe(duration.Seconds())
		slog.Debug(fmt.Sprintf("STAT: %s elapse: %s", op.desc, duration))
	}
	return nil
}
