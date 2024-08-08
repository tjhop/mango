package inventory

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	commonMetricLabels = []string{"inventory", "component"}

	// prometheus metrics
	metricMangoInventoryInfoLabels = prometheus.Labels{
		"hostname":       "unknown",
		"enrolled":       "false",
		"inventory_path": "unknown",
	}

	metricMangoInventoryInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_info",
			Help: "A metric with a constant '1' value with labels for information about the mango inventory",
		},
		[]string{"hostname", "enrolled", "inventory_path"},
	)

	metricInventory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory",
			Help: "Number items in each component of the inventory",
		},
		commonMetricLabels,
	)

	metricInventoryApplicable = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_applicable",
			Help: "Number items in each component of the inventory that are applicable to this system",
		},
		commonMetricLabels,
	)

	metricInventoryReloadSeconds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_reload_seconds",
			Help: "Unix timestamp of the last successful mango inventory reload",
		},
		commonMetricLabels,
	)

	metricInventoryReloadTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_inventory_reload_total",
			Help: "Total number of times the mango inventory has been reloaded",
		},
		commonMetricLabels,
	)

	metricInventoryReloadFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_inventory_reload_failed_total",
			Help: "Total number of times the mango inventory has failed to reload",
		},
		commonMetricLabels,
	)
)
