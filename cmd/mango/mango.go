package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // for profiling
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/pkg/utils"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/manager"
	"github.com/tjhop/mango/internal/version"
)

const (
	programName = "mango"
	// we have fun around here.
	programNameAsciiArt = " _ __ ___    __ _  _ __    __ _   ___\n" +
		"| '_ ` _ \\  / _` || '_ \\  / _` | / _ \\\n" +
		"| | | | | || (_| || | | || (_| || (_) |\n" +
		"|_| |_| |_| \\__,_||_| |_| \\__, | \\___/\n" +
		"                          |___/\n"
	defaultPrometheusPort = 9555
	charitywareMsg        = "\nMango is charityware, in honor of Bram Moolenaar and out of respect for Vim. You can use and copy it as much as you like, but you are encouraged to make a donation for needy children in Uganda.  Please visit the ICCF web site, available at these URLs:\n\nhttps://iccf-holland.org/\nhttps://www.vim.org/iccf/\nhttps://www.iccf.nl/"
)

var (
	metricMangoRuntimeInfoLabels = prometheus.Labels{
		"auto_reload": "disabled",
		"log_level":   "info",
	}

	metricMangoRuntimeInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_runtime_info",
			Help: "A metric with a constant '1' value with labels for inventory auto-reload status, logging level, etc.",
		},
		[]string{"auto_reload", "log_level"},
	)
)

func init() {
	// expose build info metric
	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "mango_build_info",
			Help: "A metric with a constant '1' value with labels for version, commit and build_date from which mango was built.",
			ConstLabels: prometheus.Labels{
				"version":    version.Version,
				"commit":     version.Commit,
				"build_date": version.BuildDate,
				"goversion":  runtime.Version(),
			},
		},
		func() float64 { return 1 },
	)
}

func mango(ctx context.Context, logger *slog.Logger, inventoryPath, hostname string) {
	metricMangoRuntimeInfo.With(metricMangoRuntimeInfoLabels).Set(1)

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

	// start manager, reload it with data from inventory, and then start a run of everything for the system
	managerLogger := logger.With(slog.String("worker", "manager"))
	managerLogger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Initializing mango manager",
	)
	mgr := manager.NewManager(hostname)

	managerLogger.LogAttrs(
		ctx,
		slog.LevelInfo,
		"Starting initial run of all modules",
	)
	mgr.ReloadAndRunAll(ctx, managerLogger, inv)

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
						mgr.ReloadAndRunAll(ctx, managerLogger, inv)
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

					// update runtime info metric
					metricMangoRuntimeInfoLabels["auto_reload"] = dur.String()
					metricMangoRuntimeInfo.With(metricMangoRuntimeInfoLabels).Set(1)

					ticker := time.NewTicker(dur)

					for {
						select {
						case <-ticker.C:
							logger.LogAttrs(
								ctx,
								slog.LevelInfo,
								"Inventory auto-reload signal received, reloading inventory and rerunning modules",
							)
							mgr.ReloadAndRunAll(ctx, managerLogger, inv)
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
	{
		// web server for metrics/pprof
		cancel := make(chan struct{})

		viper.SetDefault("metrics.port", defaultPrometheusPort)
		iface := viper.GetString("metrics.interface")
		port := viper.GetInt("metrics.port")
		address := fmt.Sprintf("%s:%d", iface, port)

		metricsServer := &http.Server{
			Addr:         address,
			Handler:      nil,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  5 * time.Second,
		}
		http.Handle("/metrics", promhttp.Handler())

		g.Add(
			func() error {
				if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
					logger.LogAttrs(
						ctx,
						slog.LevelError,
						"Mango failed to open HTTP server for metrics",
						slog.String("err", err.Error()),
						slog.String("address", address),
					)
					return err
				}

				<-cancel

				return nil
			},
			func(error) {
				if err := metricsServer.Shutdown(ctx); err != nil {
					// Error from closing listeners, or context timeout:
					logger.LogAttrs(
						ctx,
						slog.LevelError,
						"Failed to close HTTP server",
						slog.String("err", err.Error()),
					)
				}
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
	// create root logger with default configs, parse out updated configs from flags
	logLevel := new(slog.LevelVar) // default to info level logging
	logHandlerOpts := &slog.HandlerOptions{
		Level: logLevel,
	}
	logHandler := slog.NewTextHandler(os.Stdout, logHandlerOpts) // use logfmt handler by default
	logger := slog.New(logHandler)
	rootCtx := context.Background()

	// prep and parse flags
	flag.StringP("inventory.path", "i", "", "Path to mango configuration inventory")
	flag.String("inventory.reload-interval", "", "Time duration for how frequently mango will auto reload and apply the inventory [default disabled]")
	flag.StringP("logging.level", "l", "", "Logging level may be one of: [trace, debug, info, warning, error, fatal and panic]")
	flag.String("logging.output", "logfmt", "Logging format may be one of: [logfmt, json]")
	flag.String("hostname", "", "(Requires root) Custom hostname to use [default is system hostname]")
	flag.Bool("manager.skip-apply-on-test-success", false, "If enabled, this will allow mango to skip running the module's idempotent `apply` script if the `test` script passes without issues")
	flag.BoolP("help", "h", false, "Prints help and usage information")
	flag.BoolP("version", "v", false, "Prints version and build info")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\nUsage of %s:\n", programNameAsciiArt, os.Args[0])
		flag.PrintDefaults()
	}

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

	if viper.GetBool("help") {
		flag.Usage()
		fmt.Fprintln(os.Stderr, charitywareMsg)
		os.Exit(0)
	}

	if viper.GetBool("version") {
		fmt.Println(version.Print(programName))
		os.Exit(0)
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
		logLevel.Set(slog.LevelInfo)
		logger.LogAttrs(
			rootCtx,
			slog.LevelWarn,
			"Failed to parse log level from flag, defaulting to <info> level",
			slog.String("err", "Unsupported log level"),
			slog.String("log_level", logLevelFlagVal),
		)
	}

	// update runtime info metric
	metricMangoRuntimeInfoLabels["log_level"] = strings.ToLower(logLevel.Level().String())
	metricMangoRuntimeInfo.With(metricMangoRuntimeInfoLabels).Set(1)

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

	mainLogger := logger.With(
		slog.Group("hostname",
			slog.String("system", utils.GetHostname()),
			slog.String("inventory", me),
		),
	)

	mainLogger.LogAttrs(
		rootCtx,
		slog.LevelInfo,
		"Mango build information",
		slog.Group("build",
			slog.String("version", version.Version),
			slog.String("build_date", version.BuildDate),
			slog.String("commit", version.Commit),
			slog.String("go_version", runtime.Version()),
		),
	)

	// set mainLogger as default
	slog.SetDefault(mainLogger)

	// run mango daemon
	mango(rootCtx, mainLogger, inventoryPath, me)
}
