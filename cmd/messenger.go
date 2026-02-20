package cmd

import (
	"fmt"
	"log/slog"
	"net/url"

	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth/strategies/union"
	"github.com/spf13/viper"

	"github.com/opensvc/oc3/messenger"
	"github.com/opensvc/oc3/xauth"
)

type (
	messengerT struct {
		section string
	}
)

func (t *messengerT) Section() string { return t.section }

func (t *messengerT) authMiddleware(publicPath, publicPrefix []string) echo.MiddlewareFunc {
	return AuthMiddleware(union.New(
		xauth.NewPublicStrategy(publicPath, publicPrefix),
	))
}

func startMessenger() error {
	if err := setup(sectionMessenger); err != nil {
		return err
	}
	t := &messengerT{section: sectionMessenger}
	return t.run()
}

func (t *messengerT) run() error {
	section := t.Section()
	u, err := url.Parse(viper.GetString(section + ".url"))
	if err != nil {
		slog.Warn(fmt.Sprintf("parsing %s.url: %v", section, err))
		return err
	}

	if ok, errC := start(t); ok {
		slog.Info(fmt.Sprintf("%s started", section))
		go func() {
			if err := <-errC; err != nil {
				slog.Error(fmt.Sprintf("%s stopped: %s", t.Section(), err))
			}
		}()
	}

	cometCmd := messenger.CmdComet{
		Address:      u.Hostname(),
		Port:         u.Port(),
		Key:          viper.GetString(sectionMessenger + ".key"),
		RequireToken: viper.GetBool(sectionMessenger + ".require_token"),
		CertFile:     viper.GetString(sectionMessenger + ".cert_file"),
		KeyFile:      viper.GetString(sectionMessenger + ".key_file"),
	}

	return cometCmd.Run()
}
