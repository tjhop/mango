package inventory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

var (
	// prometheus metrics
	metricManagerScriptRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_script_run_seconds",
			Help: "Timestamp of the last run of the given module script, in seconds since the epoch",
		},
		[]string{"module", "run"},
	)

	metricManagerScriptRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_script_run_success_seconds",
			Help: "Timestamp of the last successful run of the given module script, in seconds since the epoch",
		},
		[]string{"module", "run"},
	)

	metricManagerScriptRunDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_script_run_duration_seconds",
			Help: "Approximately how long it took for the script to run, in seconds",
		},
		[]string{"module", "run"},
	)

	metricManagerScriptRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_script_run_total",
			Help: "A count of the total number of runs that have been performed to manage the module",
		},
		[]string{"module", "run"},
	)

	metricManagerScriptRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_script_run_failed_total",
			Help: "A count of the total number of failed runs that have been performed to manage the module",
		},
		[]string{"module", "run"},
	)
)

// RunStat tracks various runtime information about the script.
// - ExitCode: numeric exit code received from the script
// - LastRunTimestamp: timestamp of the last time a run was started for this script
// - LastRunSuccessTimestamp: timestamp of the last successful run for this script
// - RunCount: how many times this script has been run
// - FailCount: how many times this script has been fun and failed
type RunStat struct {
	ExitCode                int
	LastRunTimestamp        time.Time
	LastRunSuccessTimestamp time.Time
	LastRunDuration         time.Duration
	RunCount                int
	FailCount               int
}

// Script contains fields that are relevant to all of the executable scripts mango will be working with.
// - ID: string identifying the script (the name of the script)
// - Path: Absolute path to the script
type Script struct {
	ID    string
	Path  string
	Stats RunStat
}

// String is a stringer to return tse script ID
func (s Script) String() string { return s.ID }

// Run is responsible for actually building and dispacting the script to be
// run. After the script is finished running, it updates Stats for the script.
func (s *Script) Run(ctx context.Context) error {
	// TODO: set env variables/template script
	cmd := exec.CommandContext(ctx, s.Path)

	// set runtime directory
	tmpDir := viper.GetString("mango.temp-dir")
	cmd.Dir = tmpDir

	start := time.Now()
	parent := filepath.Base(s.Path)

	// log stdout from script
	// `$logDir/mango_$parent_$scriptID_timestamp_stdout.log`
	// eg, `/var/log/mango/mango_test-module_apply_123456_stdout.log`
	// TODO: I feel like these keys should be getting pulled from the context at this phase of things...
	logNameBase := filepath.Join(viper.GetString("mango.log-dir"), "mango_"+parent+"_"+s.ID+"_"+fmt.Sprintf("%d", start.Unix()))
	stdoutLog, err := os.OpenFile(logNameBase+"_stdout.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  stdoutLog,
		}).Error("Failed to open script log for stdout")
	}
	cmd.Stdout = stdoutLog

	// log stderr from script
	// `mango_$scriptID_timestamp_stderr.log
	stderrLog, err := os.OpenFile(logNameBase+"_stderr.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  stderrLog,
		}).Error("Failed to open script log for stderr")
	}
	cmd.Stderr = stderrLog

	defer func() {
		// close logs
		defer stdoutLog.Close()
		defer stderrLog.Close()

		// update stats
		s.Stats.LastRunDuration = time.Now().Sub(start)
		s.Stats.RunCount++
		s.Stats.LastRunTimestamp = start

		// update metrics
		metricManagerScriptRunTimestamp.With(prometheus.Labels{"module": parent, "run": s.ID}).Set(float64(start.Unix()))
		metricManagerScriptRunDuration.With(prometheus.Labels{"module": parent, "run": s.ID}).Set(s.Stats.LastRunDuration.Seconds())
		metricManagerScriptRunTotal.With(prometheus.Labels{"module": parent, "run": s.ID}).Inc()

		log.WithFields(log.Fields{
			"path":     s.Path,
			"duration": s.Stats.LastRunDuration,
		}).Debug("Script run finished")
	}()

	err = cmd.Run()

	// inspired by https://stackoverflow.com/questions/10385551/get-exit-code-go/62647366#62647366
	var (
		ee *exec.ExitError
		pe *os.PathError
	)

	if err != nil {
		s.Stats.FailCount++
		metricManagerScriptRunFailedTotal.With(prometheus.Labels{"module": parent, "run": s.ID}).Inc()

		if errors.As(err, &ee) {
			exitCode := ee.ExitCode()

			s.Stats.ExitCode = exitCode

			log.WithFields(log.Fields{
				"path":      s.Path,
				"error":     ee,
				"exit_code": exitCode,
			}).Error("Script Run Failed")
		} else if errors.As(err, &pe) {
			s.Stats.ExitCode = 0

			log.WithFields(log.Fields{
				"path":  s.Path,
				"error": pe,
			}).Error("Script Run Failed")
		} else {
			s.Stats.ExitCode = 0

			log.WithFields(log.Fields{
				"path":  s.Path,
				"error": err,
			}).Error("Script Run Failed")
		}

		return err
	}

	// no err == exit code 0
	s.Stats.ExitCode = 0
	s.Stats.LastRunSuccessTimestamp = start
	metricManagerScriptRunSuccessTimestamp.With(prometheus.Labels{"module": parent, "run": s.ID}).Set(float64(start.Unix()))

	return nil
}
