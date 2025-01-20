package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tjhop/mango/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logLevel = &slog.LevelVar{}
	rootCmd  = &cobra.Command{
		Use:     "mh",
		Short:   "Mango Helper -- Tool to work with Mango",
		Long:    "Mango Helper is a utility tool to aid in working with mango",
		Version: version.Print(os.Args[0]),
	}
)

func init() {
	rootCmdFlagSet := rootCmd.PersistentFlags()
	rootCmdFlagSet.StringP("logging.level", "l", "info", "Logging level may be one of: [debug, info, warning, error]")
	rootCmdFlagSet.String("logging.output", "logfmt", "Logging format may be one of: [logfmt, json]")
	if err := viper.BindPFlags(rootCmdFlagSet); err != nil {
		panic(fmt.Errorf("Error binding flags for command <%s>: %w", "mh", err))
	}

	logHandlerOpts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			key := a.Key
			switch key {
			case slog.SourceKey:
				src, _ := a.Value.Any().(*slog.Source)
				a.Value = slog.StringValue(filepath.Base(src.File) + ":" + strconv.Itoa(src.Line))
			default:
			}

			return a
		},
	}

	// parse log output format from flag, create root logger with default configs
	var logger *slog.Logger
	logOutputFormat := strings.ToLower(viper.GetString("logging.output"))
	if logOutputFormat == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, logHandlerOpts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, logHandlerOpts))
	}

	// parse log level from flag
	logLevelFlagVal := strings.ToLower(viper.GetString("logging.level"))
	switch logLevelFlagVal {
	case "":
		logLevel.Set(slog.LevelInfo)
		logger.Warn("Log level flag not set, defaulting to <info> level")
	case "info": // default is info, we're good
	case "warn":
		logLevel.Set(slog.LevelWarn)
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
		logger.Warn("Failed to parse log level from flag, defaulting to <info> level",
			slog.String("err", "Unsupported log level"),
			slog.String("log_level", logLevelFlagVal),
		)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("Error running root cobra command", "err", err)
		os.Exit(1)
	}
}
