package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	_ "github.com/tjhop/mango/internal/logging"
	"github.com/tjhop/mango/internal/mango"
	"github.com/tjhop/mango/internal/metrics"
)

const (
	programName = "mango"
)

func run(ctx context.Context) error {
	log.Info("Mango server started")
	defer log.Info("Mango server finished")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go metrics.ExportPrometheusMetrics()

	log.Info("Initializing mango tree to track configs")
	mango.InitTree()

	for {
		select {
		case sig := <-sigs:
			log.WithFields(log.Fields{
				"signal": sig,
			}).Info("Caught signal, waiting for work to finish and terminating")

			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func main() {
	// prep and parse flags
	flag.String("config", "", "Path to configuration file to use")
	flag.String("mango.tree", "", "Path to mango configuration tree")
	flag.String("logging.level", "", "Logging level may be one of: trace, debug, info, warning, error, fatal and panic")

	flag.Parse()
	viper.BindPFlags(flag.CommandLine)

	// prep and read config file
	home, err := homedir.Dir()
	if err != nil {
		// log and continue on, home directory retreival doesn't have to be a hard failure
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to retreive home directory when checking for configuration files")
	}

	viper.SetConfigName(programName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(filepath.Join("/etc", programName))
	viper.AddConfigPath(filepath.Join(home, programName))
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to read configuration file")
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		log.WithFields(log.Fields{
			"file": e.Name,
		}).Info("Mango config reloaded")
	})
	viper.WatchConfig()

	// set log level based on config
	level, err := log.ParseLevel(viper.GetString("logging.level"))
	if err != nil {
		// if log level couldn't be parsed from config, default to info level
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(level)
		log.Infof("Log level set to: %s", level)
	}

	// run mango daemon
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Mango server recieved error")
	}
}
