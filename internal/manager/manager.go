package manager

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/tjhop/mango/internal/inventory"
)

var (
	// prometheus metrics
	metricManagerRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_run_seconds",
			Help: "Timestamp of the last run of the given module, in seconds since the epoch",
		},
		[]string{"module", "run"},
	)

	metricManagerRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_run_success_seconds",
			Help: "Timestamp of the last successful run of the given module, in seconds since the epoch",
		},
		[]string{"module", "run"},
	)

	metricManagerRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_run_total",
			Help: "A count of the total number of runs that have been performed to manage the module",
		},
		[]string{"module", "run"},
	)

	metricManagerRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_run_failed_total",
			Help: "A count of the total number of runs that have been performed to manage the module",
		},
		[]string{"module", "run"},
	)
)

// Manager contains fields related to track and execute runnable modules and statistics.
type Manager struct {
	id         string
	modules    []inventory.Module
	directives []inventory.DirectiveScript
}

func (m *Manager) String() string { return m.id }

// NewManager returns a new Manager struct instantiated with the given ID
func NewManager(id string) Manager {
	return Manager{id: id}
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (m *Manager) Reload(inv inventory.Store) {
	m.modules = inv.GetModulesForSelf()
	m.directives = inv.GetDirectivesForSelf()
}
