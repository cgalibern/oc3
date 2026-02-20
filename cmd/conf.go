package cmd

import (
	"strings"

	"github.com/spf13/viper"

	"github.com/opensvc/oc3/oc2websocket"
)

var (
	configCandidateDirs = []string{"/etc/oc3/", "$HOME/.config/oc3", "./"}
)

func workerSection(name string) string {
	if name != "" {
		return sectionWorker + "." + name
	} else {
		return sectionWorker
	}
}

func setDefaultWorkerConfig(name string) {
	section := workerSection(name)
	uxSocket := "/var/run/oc3_worker_pprof.sock"
	if name != "" {
		uxSocket = "/var/run/oc3_worker_" + name + "_pprof.sock"
	}
	viper.SetDefault(section+".addr", "127.0.0.1:8100")
	viper.SetDefault(section+".pprof.net.enable", false)
	viper.SetDefault(section+".pprof.ux.enable", false)
	viper.SetDefault(section+".pprof.ux.socket", uxSocket)
	viper.SetDefault(section+".metrics.enable", false)

	viper.SetDefault(section+".runners", 1)
	viper.SetDefault(section+".tx", true)
}

func setDefaultFeederConfig() {
	s := sectionFeeder
	viper.SetDefault(s+".addr", "127.0.0.1:8080")
	viper.SetDefault(s+".pprof.uxsocket", "/var/run/oc3_feeder_pprof.sock")
	viper.SetDefault(s+".pprof.net.enable", false)
	viper.SetDefault(s+".pprof.ux.enable", false)
	viper.SetDefault(s+".pprof.ux.socket", "/var/run/oc3_feeder_pprof.sock")
	viper.SetDefault(s+".pprof.enable", false)
	viper.SetDefault(s+".metrics.enable", false)
	viper.SetDefault(s+".ui.enable", false)
	viper.SetDefault(s+".sync.timeout", "2s")
}

func setDefaultServerConfig() {
	s := sectionServer
	viper.SetDefault(s+".addr", "127.0.0.1:8081")
	viper.SetDefault(s+"pprof.pprof.net.enable", false)
	viper.SetDefault(s+"pprof.pprof.ux.enable", false)
	viper.SetDefault(s+"pprof.pprof.ux.socket", "/var/run/oc3_server_pprof.sock")
	viper.SetDefault(s+".metrics.enable", false)
	viper.SetDefault(s+".ui.enable", false)
	viper.SetDefault(s+".sync.timeout", "2s")
	viper.SetDefault(s+".allow_anon_register", false)
}

func setDefaultSchedulerConfig() {
	s := sectionScheduler
	viper.SetDefault(s+".addr", "127.0.0.1:8082")
	viper.SetDefault(s+".pprof.net.enable", false)
	viper.SetDefault(s+".pprof.ux.enable", false)
	viper.SetDefault(s+".pprof.ux.socket", "/var/run/oc3_scheduler_pprof.sock")
	viper.SetDefault(s+".metrics.enable", false)
	viper.SetDefault(s+".task.trim.retention", 365)
	viper.SetDefault(s+".task.trim.batch_size", 1000)
}

func setDefaultMessengerConfig() {
	s := sectionMessenger
	viper.SetDefault(s+".addr", "127.0.0.1:8083")
	viper.SetDefault(s+".pprof.net.enable", false)
	viper.SetDefault(s+".pprof.ux.enable", false)
	viper.SetDefault(s+".pprof.ux.socket", "/var/run/oc3_messenger_pprof.sock")
	viper.SetDefault(s+".metrics.enable", false)
	viper.SetDefault(s+".key", "magix123")
	viper.SetDefault(s+".url", "http://127.0.0.1:8889")
	viper.SetDefault(s+".require_token", false)
	viper.SetDefault(s+".key_file", "")
	viper.SetDefault(s+".cert_file", "")

	setDefaultAuthConfig()
}

func setDefaultRunnerConfig() {
	s := sectionRunner
	viper.SetDefault(s+".addr", "127.0.0.1:8084")
	viper.SetDefault(s+".pprof.net.enable", false)
	viper.SetDefault(s+".pprof.ux.enable", false)
	viper.SetDefault(s+".pprof.ux.socket", "/var/run/oc3_runner_pprof.sock")
	viper.SetDefault(s+".metrics.enable", false)
}

func setDefaultDBConfig() {
	viper.SetDefault("db.username", "opensvc")
	viper.SetDefault("db.host", "127.0.0.1")
	viper.SetDefault("db.port", "3306")
	viper.SetDefault("db.log.level", "warn")
	viper.SetDefault("db.log.slow_query_threshold", "1s")
}

func setDefaultRedisConfig() {
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.password", "")
}

func setDefaultAuthConfig() {
	viper.SetDefault("w2p_hmac", "sha512:7755f108-1b83-45dc-8302-54be8f3616a1")
}

func initConfig() error {
	// env
	viper.SetEnvPrefix("OC3")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// defaults
	setDefaultDBConfig()
	setDefaultRedisConfig()
	setDefaultFeederConfig()
	setDefaultServerConfig()
	setDefaultSchedulerConfig()
	setDefaultMessengerConfig()
	setDefaultMessengerConfig()
	setDefaultRunnerConfig()

	// config file
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	for _, d := range configCandidateDirs {
		viper.AddConfigPath(d)
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		} else {
			return err
		}
	}
	return nil
}

func newEv() *oc2websocket.T {
	return &oc2websocket.T{
		Url: viper.GetString("messenger.url"),
		Key: []byte(viper.GetString("messenger.key")),
	}
}
