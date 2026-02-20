package cmd

import (
	"context"
	"database/sql"

	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"
	"github.com/spf13/viper"

	api "github.com/opensvc/oc3/server"
	handlers "github.com/opensvc/oc3/server/handlers"
	"github.com/opensvc/oc3/xauth"
)

type (
	server struct {
		db      *sql.DB
		section string
		redis   *redis.Client
	}
)

func newServer() (*server, error) {
	if db, err := newDatabase(); err != nil {
		return nil, err
	} else {
		return &server{db: db, section: sectionServer, redis: newRedis()}, nil
	}
}

func (t *server) Section() string { return t.section }

func (t *server) apiRegister(e *echo.Echo) {
	api.RegisterHandlersWithBaseURL(e, &handlers.Api{
		DB:          t.db,
		Redis:       t.redis,
		UI:          viper.GetBool(t.section + ".ui.enable"),
		SyncTimeout: viper.GetDuration(t.section + ".sync.timeout"),
	}, pathApi)
}

func (t *server) docMiddleware() echo.MiddlewareFunc {
	return handlers.UIMiddleware(context.Background(), pathApi, pathSpec)
}

func (t *server) authMiddleware(publicPath, publicPrefix []string) echo.MiddlewareFunc {
	return handlers.AuthMiddleware(union.New(
		xauth.NewPublicStrategy(publicPath, publicPrefix),
		xauth.NewBasicWeb2py(t.db, viper.GetString("w2p_hmac")),
		xauth.NewBasicNode(t.db),
	))
}
