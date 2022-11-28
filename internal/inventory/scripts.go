package inventory

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

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
			Help: "A count of the total number of runs that have been performed to manage the module",
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

// Run is responsible for actually building and dispacting the script to be
// run. After the script is finished running, it updates Stats for the script.
func (s *Script) Run(ctx context.Context) error {
	// TODO: set working directory to mango temp dir prior to execution
	// TODO: update metrics here or in manager?
	// TODO: set env variables/template script
	// TODO: allow for logging to files?
	cmd := exec.CommandContext(ctx, s.Path)
	start := time.Now()
	parent := filepath.Base(s.Path)

	defer func() {
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

	// TODO: is there ever a reason/benefit to using `exec.Start()` / `exec.Wait()`?
	// actually run the command
	err := cmd.Run()

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
