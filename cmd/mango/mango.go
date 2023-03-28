package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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

func mango(inventoryPath, hostname string) {
	logger := log.WithFields(log.Fields{
		"version":    config.Version,
		"build_date": config.BuildDate,
		"commit":     config.Commit,
		"go_version": self.GetRuntimeVersion(),
	})
	logger.Info("Mango server started")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricMangoRuntimeInfo.With(prometheus.Labels{"hostname": hostname}).Set(1)
	metricServiceStartSeconds.Set(float64(time.Now().Unix()))

	// create directory for persistent logs
	logDir := filepath.Join("/var/log", programName)
	err := os.MkdirAll(logDir, 0755)
	if err != nil && !os.IsExist(err) {
		logger.WithFields(log.Fields{
			"err":  err,
			"path": logDir,
		}).Fatal("Failed to create persistent directory for logs")
	}
	viper.Set("mango.log-dir", logDir)

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
		}).Fatal("Failed to create temporary directory for mango")
	}
	viper.Set("mango.temp-dir", dir)

	// serve metrics
	go metrics.ExportPrometheusMetrics()

	// load inventory
	log.WithFields(log.Fields{
		"path": inventoryPath,
	}).Info("Initializing mango inventory")
	inv := inventory.NewInventory(inventoryPath, hostname)
	// reload inventory
	inv.Reload(ctx)
	log.WithFields(log.Fields{
		"enrolled": inv.IsEnrolled(),
	}).Info("Host enrollment check")

	// start manager
	log.WithFields(log.Fields{
		"manager": hostname,
	}).Info("Initializing mango manager")
	mgr := manager.NewManager(hostname)
	// reload manager
	mgr.Reload(ctx, inv)

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
						inv.Reload(ctx)
						mgr.Reload(ctx, inv)

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
	defer func() {
		cleanup()
		logger.Info("Mango server finished")
	}()
	if err := g.Run(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Mango server received error")
	}
}

// cleanup contains anything that needs to be run prior to mango gracefully
// shutting down
func cleanup() {
	log.Debug("Cleaning up prior to exit")

	tmpDir := viper.GetString("mango.temp-dir")
	if err := os.RemoveAll(tmpDir); err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  tmpDir,
		}).Error("Failed to remove temporary directory")
	}
}

func main() {
	// prep and parse flags
	flag.StringP("inventory.path", "i", "", "Path to mango configuration inventory")
	flag.StringP("logging.level", "l", "", "Logging level may be one of: [trace, debug, info, warning, error, fatal and panic]")
	flag.String("logging.output", "logfmt", "Logging format may be one of: [logfmt, json]")
	flag.String("hostname", "", "Custom hostname to use (default's to system hostname if unset)")

	flag.Parse()
	if err := viper.BindPFlags(flag.CommandLine); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to parse command line flags")
	}

	logOutputFormat := strings.ToLower(viper.GetString("logging.output"))
	if logOutputFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{})
	}

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
			logPrettyfierFunc := func(f *runtime.Frame) (string, string) {
				fileName := filepath.Base(f.File)
				funcName := filepath.Base(f.Function)
				return fmt.Sprintf("%s()", funcName), fmt.Sprintf("%s:%d", fileName, f.Line)
			}

			if logOutputFormat == "json" {
				log.SetFormatter(&log.JSONFormatter{CallerPrettyfier: logPrettyfierFunc})
			} else {
				log.SetFormatter(&log.TextFormatter{CallerPrettyfier: logPrettyfierFunc})
			}
		}

		log.Infof("Log level set to: %s", level)
	}

	// ensure inventory is set
	inventoryPath := viper.GetString("inventory.path")
	if inventoryPath == "" {
		log.Fatal("Inventory not defined, please set `inventory.path` flag or config variable to the path to the inventory")
	}

	// run mango daemon
	me := viper.GetString("hostname")
	if me == "" {
		me = self.GetHostname()
	}
	mango(inventoryPath, me)
}
