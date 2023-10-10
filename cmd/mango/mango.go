package main

import (
	"context"
	"log/slog"
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
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/pkg/utils"

	"github.com/tjhop/mango/internal/config"
	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/manager"
	"github.com/tjhop/mango/internal/metrics"
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

	// TODO: @tjhop move `enrolled` to an inventory pkg metric
	// TODO: @tjhop move `manager` to a manager pkg metric
	// TODO: @tjhop add labels for: [auto_reload: true|false]
	metricMangoRuntimeInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_runtime_info",
			Help: "A metric with a constant '1' value with labels for information about the mango runtime, such as system hostname.",
		},
		[]string{"hostname", "enrolled", "manager"},
	)
)

func mango(ctx context.Context, logger *slog.Logger, inventoryPath, hostname string) {
	mangoStart := time.Now()
	metricServiceStartSeconds.Set(float64(mangoStart.Unix()))

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		cleanup(ctx, logger)
		logger.LogAttrs(
			ctx,
			slog.LevelInfo,
			"Mango server finished",
		)
	}()

	logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Mango server started",
	)

	// create directory for persistent logs
	logDir := filepath.Join("/var/log", programName)
	err := os.MkdirAll(logDir, 0755)
	if err != nil && !os.IsExist(err) {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to create persistent directory for logs",
			slog.String("err", err.Error()),
		)
		os.Exit(1)
	}
	viper.Set("mango.log-dir", logDir)

	// create ephemeral directory for mango to store temporary files
	tmpDir := viper.GetString("mango.temp-dir")
	logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Creating temporary directory for mango runtime files",
		slog.String("path", tmpDir),
	)

	dir, err := os.MkdirTemp(tmpDir, programName)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to create temporary directory for mango",
			slog.String("err", err.Error()),
			slog.String("path", tmpDir),
		)
		os.Exit(1)
	}
	viper.Set("mango.temp-dir", dir)

	// serve metrics
	go metrics.ExportPrometheusMetrics()

	// load inventory
	inventoryLogger := logger.With(
		slog.String("worker", "inventory"),
		slog.Group(
			"inventory",
			slog.String("path", inventoryPath),
		),
	)
	inventoryLogger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Initializing mango inventory",
	)
	inv := inventory.NewInventory(inventoryPath, hostname)
	// reload inventory
	inv.Reload(ctx, inventoryLogger)
	// enrolled := inv.IsEnrolled()

	// start manager, reload it with data from inventory, and then start a run of everything for the system
	logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Initializing mango manager",
		slog.String("manager", hostname),
	)
	mgr := manager.NewManager(hostname)

	// // TODO(@tjhop): consider reworking this runtime var to be inventory
	// // runtime info and move it to inventory pkg?
	// metricMangoRuntimeInfo.With(prometheus.Labels{
	// 	// TODO(@tjhop): include system/given hostname, similar to log on l384
	// 	"hostname": hostname,
	// 	"enrolled": strconv.FormatBool(enrolled),
	// }).Set(1)
	// logger = logger.With(
	// 	slog.Group("hostname",
	// 	slog.String("hostname", hostname),
	// 	slog.Bool("enrolled", enrolled),
	// 	),
	// )
	// logger.LogAttrs(
	// 	ctx,
	// 	slog.LevelInfo,
	// 	"Host enrollment check",
	// )

	logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Starting initial run of all modules",
	)
	mgr.ReloadAndRunAll(ctx, inv)

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
					logger.LogAttrs(
						ctx,
						slog.LevelWarn,
						"Caught signal, waiting for work to finish and terminating",
						slog.String("signal", sig.String()),
					)

					// cancel context, triggering a
					// cancelation of everything using it
					// (including manager and scripts)
					cancel()
				case <-ctx.Done():
					if err := ctx.Err(); err != nil {
						logger.LogAttrs(
							ctx,
							slog.LevelError,
							"Context canceled due to error",
							slog.String("err", err.Error()),
						)
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
						logger.LogAttrs(
							ctx,
							slog.LevelWarn,
							"Caught signal, reloading configuration and inventory",
							slog.String("signal", sig.String()),
						)

						// reload inventory
						inv.Reload(ctx, inventoryLogger)

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
				// block and wait for reload/close signals from channels
				for {
					select {
					case <-reloadCh:
						// when a signal is received on the
						// reload channel, trigger a new run
						// for all modules.
						mgr.ReloadAndRunAll(ctx, inv)
					case <-cancel:
						return nil
					}
				}
			},
			func(error) {
				close(cancel)
			},
		)
	}
	{
		// ticker routine for auto reload, if configured
		cancel := make(chan struct{})
		g.Add(
			func() error {
				interval := viper.GetString("inventory.reload-interval")
				if interval == "" {
					// auto update not enabled, log and carry on
					logger.LogAttrs(
						ctx,
						slog.LevelInfo,
						"Inventory auto-reload is not enabled, mango will only re-apply inventory if sent a SIGHUP",
					)
					<-cancel
				} else {
					// auto update enabled, attempt to configure or carry on
					dur, err := time.ParseDuration(interval)
					if err != nil {
						logger.LogAttrs(
							ctx,
							slog.LevelError,
							"Failed to parse duration for inventory auto-reload, continuing without enabling",
							slog.String("err", err.Error()),
						)

						return nil
					}

					logger.LogAttrs(
						ctx,
						slog.LevelInfo,
						"Inventory auto-reload enabled",
						slog.String("interval", dur.String()),
					)

					ticker := time.NewTicker(dur)

					for {
						select {
						case <-ticker.C:
							logger.LogAttrs(
								ctx,
								slog.LevelInfo,
								"Inventory auto-reload signal received, reloading inventory and rerunning modules",
							)
							mgr.ReloadAndRunAll(ctx, inv)
						case <-cancel:
							return nil
						}
					}
				}

				return nil
			},
			func(error) {
				close(cancel)
			},
		)
	}

	logger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Mango server ready",
	)
	if err := g.Run(); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Mango server received error",
			slog.String("err", err.Error()),
		)
	}
}

// cleanup contains anything that needs to be run prior to mango gracefully
// shutting down
func cleanup(ctx context.Context, logger *slog.Logger) {
	logger.LogAttrs(
		ctx,
		slog.LevelDebug,
		"Cleaning up prior to exit",
	)

	tmpDir := viper.GetString("mango.temp-dir")
	if err := os.RemoveAll(tmpDir); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to remove temporary directory",
			slog.String("err", err.Error()),
			slog.String("path", tmpDir),
		)
	}
}

func main() {
	// prep and parse flags
	flag.StringP("inventory.path", "i", "", "Path to mango configuration inventory")
	flag.String("inventory.reload-interval", "", "Time duration for how frequently mango will auto reload and apply the inventory [default disabled]")
	flag.StringP("logging.level", "l", "", "Logging level may be one of: [trace, debug, info, warning, error, fatal and panic]")
	flag.String("logging.output", "logfmt", "Logging format may be one of: [logfmt, json]")
	flag.String("hostname", "", "(Requires root) Custom hostname to use [default is system hostname]")

	// create root logger with default configs, parse out updated configs from flags
	logLevel := new(slog.LevelVar) // default to info level logging
	logHandlerOpts := &slog.HandlerOptions{
		Level: logLevel,
	}
	logHandler := slog.NewTextHandler(os.Stdout, logHandlerOpts) // use logfmt handler by default
	logger := slog.New(logHandler)
	rootCtx := context.Background()

	flag.Parse()
	if err := viper.BindPFlags(flag.CommandLine); err != nil {
		logger.LogAttrs(
			rootCtx,
			slog.LevelError,
			"Failed to parse command line flags",
			slog.String("err", err.Error()),
		)
		os.Exit(1)
	}

	// parse log level from flag
	logLevelFlagVal := strings.TrimSpace(strings.ToLower(viper.GetString("logging.level")))
	switch logLevelFlagVal {
	case "info": // default is info, we're good
	case "warn":
		logLevel.Set(slog.LevelWarn)
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logger.LogAttrs(
			rootCtx,
			slog.LevelError,
			"Failed to parse log level from flag",
			slog.String("err", "Unsupported log level"),
			slog.String("level", logLevelFlagVal),
		)
		os.Exit(1)
	}

	// parse log output format from flag
	logOutputFormat := strings.TrimSpace(strings.ToLower(viper.GetString("logging.output")))
	if logOutputFormat == "json" {
		jsonLogHandler := slog.NewJSONHandler(os.Stdout, logHandlerOpts)
		logger = slog.New(jsonLogHandler)
	}

	if logger.Enabled(rootCtx, slog.LevelDebug) {
		logHandlerOpts.AddSource = true
	}

	// ensure inventory is set
	inventoryPath := viper.GetString("inventory.path")
	if inventoryPath == "" {
		logger.LogAttrs(
			rootCtx,
			slog.LevelError,
			"Failed to get inventory",
			slog.String("err", "Inventory not defined, please set `--inventory.path` flag"),
		)
		os.Exit(1)
	}

	// get hostname for inventory
	me := utils.GetHostname()

	// only allow setting custom hostname if running as root
	if os.Geteuid() == 0 {
		customHostname := viper.GetString("hostname")
		if customHostname != "" {
			me = customHostname
		}
	}

	logger = logger.With(
		slog.Group("hostname",
			slog.String("system", utils.GetHostname()),
			slog.String("inventory", me),
		),
	)

	logger.LogAttrs(
		rootCtx,
		slog.LevelInfo,
		"Mango build information",
		slog.Group("build",
			slog.String("version", config.Version),
			slog.String("build_date", config.BuildDate),
			slog.String("commit", config.Commit),
			slog.String("go_version", runtime.Version()),
		),
	)

	// set logger as default
	slog.SetDefault(logger)

	// run mango daemon
	mango(rootCtx, logger, inventoryPath, me)
}
