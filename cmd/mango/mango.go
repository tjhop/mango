package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/go-homedir"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/internal/config"
	"github.com/tjhop/mango/internal/inventory"
	_ "github.com/tjhop/mango/internal/logging"
	"github.com/tjhop/mango/internal/manager"
	"github.com/tjhop/mango/internal/metrics"
	"github.com/tjhop/mango/internal/self"
)

const (
	programName = "mango"
)

var (
	metricServiceStartSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mango_service_start_seconds",
			Help: "Unix timestamp of when mango started",
		},
	)

	metricMangoRuntimeInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_runtime_info",
			Help: "A metric with a constant '1' value with labels for information about the mango runtime, such as system hostname.",
		},
		[]string{"hostname"},
	)
)

func mango() {
	logger := log.WithFields(log.Fields{
		"version":    config.Version,
		"build_date": config.BuildDate,
		"commit":     config.Commit,
		"go_version": self.GetRuntimeVersion(),
	})
	logger.Info("Mango server started")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	me := self.GetHostname()
	metricMangoRuntimeInfo.With(prometheus.Labels{"hostname": me}).Set(1)
	metricServiceStartSeconds.Set(float64(time.Now().Unix()))

	// create ephemeral directory for mango to store temporary files
	tmpDir := viper.GetString("mango.temp-dir")
	logger.WithFields(log.Fields{
		"path": tmpDir,
	}).Info("Creating temporary directory for mango runtime files")

	dir, err := os.MkdirTemp(tmpDir, programName)
	if err != nil {
		logger.WithFields(log.Fields{
			"err":  err,
			"path": tmpDir,
			"dir":  dir,
		}).Fatal("Failed to create temporary directory for mango")
	}
	viper.Set("mango.temp-dir", dir)

	// serve metrics
	go metrics.ExportPrometheusMetrics()

	// load inventory
	inventoryPath := viper.GetString("inventory.path")
	log.WithFields(log.Fields{
		"path": inventoryPath,
	}).Info("Initializing mango inventory")
	inv := inventory.NewInventory(inventoryPath)
	// reload inventory
	inv.Reload()
	log.WithFields(log.Fields{
		"enrolled": inv.IsEnrolled(),
	}).Info("Host enrollment check")

	// start manager
	log.WithFields(log.Fields{
		"manager_id": me,
	}).Info("Initializing mango manager")
	mgr := manager.NewManager(me)
	// reload manager
	mgr.Reload(inv)

	reloadCh := make(chan struct{})
	var g run.Group
	{
		// termination and cleanup
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		g.Add(
			func() error {
				select {
				case sig := <-term:
					log.WithFields(log.Fields{
						"signal": sig,
					}).Warn("Caught signal, waiting for work to finish and terminating")

					// cancel context, triggering a
					// cancelation of everything using it
					// (including manager and scripts)
					cancel()
				case <-ctx.Done():
					if err := ctx.Err(); err != nil {
						log.WithFields(log.Fields{
							"error": err,
						}).Error("Context canceled due to error")
					}

					// close the reload -> manager run signal channel
					close(reloadCh)

					// catch-all cleanup work
					cleanup()
				}

				return nil
			},
			func(err error) {
				cancel()
			},
		)
	}
	{
		// reload handling
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				for {
					select {
					case sig := <-hup:
						log.WithFields(log.Fields{
							"signal": sig,
						}).Info("Caught signal, reloading configuration and inventory")

						// reload inventory and manager
						inv.Reload()
						mgr.Reload(inv)

						// signal the manager runner
						// goroutine that a reload
						// request has been received so
						// that it can act on it
						reloadCh <- struct{}{}
					case <-cancel:
						return nil
					}
				}
			},
			func(err error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}
	{
		// manager runner
		cancel := make(chan struct{})
		g.Add(
			func() error {
				log.Info("Starting initial run of all modules")
				mgr.RunAll(ctx)

				// block and wait for reload/close signals from channels
				for {
					select {
					case <-reloadCh:
						// when a signal is received on the
						// reload channel, trigger a new run
						// for all modules.

						// TODO(@tjhop): add logic to prevent
						// triggering a module run when one is
						// already active
						log.Info("Running all modules")
						mgr.RunAll(ctx)
					case <-cancel:
						return nil
					}
				}
			}, func(error) {
				close(cancel)
			},
		)
	}

	logger.Info("Mango server ready")
	if err := g.Run(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Mango server received error")
	}
	logger.Info("Mango server finished")
}

// cleanup contains anything that needs to be run prior to mango gracefully
// shutting down
func cleanup() {
	os.RemoveAll(viper.GetString("mango.temp-dir"))
}

func main() {
	// prep and parse flags
	flag.String("config", "", "Path to configuration file to use")
	flag.String("inventory.path", "", "Path to mango configuration inventory")
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

	configFile := viper.GetString("config")
	viper.SetConfigType("yaml")
	if configFile != "" {
		// config file set by flag, use that
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName(programName)
		viper.AddConfigPath(filepath.Join("/etc", programName))

		// if home dir was successfully retrieved, add XDG config in home dir
		if home != "" {
			viper.AddConfigPath(filepath.Join(home, ".config", programName))
		}
		viper.AddConfigPath(".")
	}

	log.WithFields(log.Fields{
		"config": viper.ConfigFileUsed(),
	}).Info("Mango config file in use")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to read configuration file")
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		log.WithFields(log.Fields{
			"config": e.Name,
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

		if level >= log.DebugLevel {
			// enable func/file logging
			log.SetReportCaller(true)
			log.SetFormatter(&log.TextFormatter{
				CallerPrettyfier: func(f *runtime.Frame) (string, string) {
					fileName := filepath.Base(f.File)
					funcName := filepath.Base(f.Function)
					return fmt.Sprintf("%s()", funcName), fmt.Sprintf("%s:%d", fileName, f.Line)
				},
			})
		}

		log.Infof("Log level set to: %s", level)
	}

	// run mango daemon
	mango()
}
