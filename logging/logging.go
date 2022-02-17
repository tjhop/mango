package logging

import (
	"fmt"
	"io/ioutil"
	"os"

	"path"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"github.com/spf13/viper"
)

func init() {
	// init logging
	log.SetOutput(ioutil.Discard) // Send all logs to nowhere by default

	log.AddHook(&writer.Hook{ // Send logs with level higher than warning to stderr
		Writer: os.Stderr,
		LogLevels: []log.Level{
			log.PanicLevel,
			log.FatalLevel,
			log.ErrorLevel,
			log.WarnLevel,
		},
	})
	log.AddHook(&writer.Hook{ // Send info and debug logs to stdout
		Writer: os.Stdout,
		LogLevels: []log.Level{
			log.InfoLevel,
			log.DebugLevel,
		},
	})

	// enable func/file logging
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			fileName := path.Base(f.File)
			funcName := path.Base(f.Function)
			return fmt.Sprintf("%s()", funcName), fmt.Sprintf("%s:%d", fileName, f.Line)
		},
	})

	level, err := log.ParseLevel(viper.GetString("logging.level"))
	if err != nil {
		// if log level couldn't be parsed from config, default to info level
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(level)
	}
}
