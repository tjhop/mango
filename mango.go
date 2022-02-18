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
	"github.com/spf13/viper"

	"github.com/tjhop/mango/logging"
)

const (
	programName = "mango"
)

func mango(ctx context.Context) error {
	log.Info("Mango server started")
	defer log.Info("Mango server finished")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigs:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func main() {
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

	// run mango daemon
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mango(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Mango server recieved error")
	}
}
