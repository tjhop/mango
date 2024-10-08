package manager

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// prometheus metrics

	// module run stat metrics
	metricManagerModuleRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_module_run_timestamp_seconds",
			Help: "Timestamp of the last run of the given module, in seconds since the epoch",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_module_run_success_timestamp_seconds",
			Help: "Timestamp of the last successful run of the given module, in seconds since the epoch",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mango_manager_module_run_duration_seconds",
			Help:    "Histogram of durations of how long it took a given module to run, in seconds",
			Buckets: prometheus.ExponentialBuckets(0.25, 2, 10),
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_module_run_total",
			Help: "A count of the total number of runs that have been performed to manage the module",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_module_run_failed_total",
			Help: "A count of the total number of failed runs that have been performed to manage the module",
		},
		[]string{"module", "script"},
	)

	// directive run stat metrics
	metricManagerDirectiveRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_timestamp_seconds",
			Help: "Timestamp of the last run of the given directive, in seconds since the epoch",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_success_timestamp_seconds",
			Help: "Timestamp of the last successful run of the given directive, in seconds since the epoch",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_duration_seconds",
			Help: "Approximately how long it took for the directive to run, in seconds",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_directive_run_total",
			Help: "A count of the total number of runs that have been performed to manage the directive",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_directive_run_failed_total",
			Help: "A count of the total number of failed runs that have been performed to manage the directive",
		},
		[]string{"directive"},
	)

	// don't add runID to run-in-progress metric -- even though it could be
	// useful, it'll hurt cardinality. Consider adding it later as a
	// trace/examplar.
	metricManagerRunInProgress = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_run_in_progress",
			Help: "A metric with a constant '1' when the named manager is actively running directives/modules",
		},
		[]string{"manager"},
	)
)
