package inventory

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/internal/self"
)

var (
	once               sync.Once
	globalInventory    Inventory
	commonMetricLabels = []string{"inventory", "component", "hostname"}

	// prometheus metrics
	metricInventoryTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_total",
			Help: "Total number items in each component of the inventory",
		},
		commonMetricLabels,
	)

	metricInventoryApplicableTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_applicable_total",
			Help: "Total number items in each component of the inventory that are applicable to this system",
		},
		commonMetricLabels,
	)

	metricInventoryReloadSeconds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_inventory_reload_seconds",
			Help: "Unix timestamp of the last successful mango inventory reload",
		},
		append(commonMetricLabels, "result"),
	)

	metricInventoryReloadTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_inventory_reload_total",
			Help: "Total number of times the mango inventory has been reloaded",
		},
		append(commonMetricLabels, "result"),
	)
)

// Inventory contains fields that comprise the data that makes up our inventory.
// - Hosts: a map of `Host` structs for each parsed host, keyed on the `Host.ID`
// - Modules: a map of `Module` structs for each parsed module, keyed on the `Module.ID`
// - Roles: a map of `Role` structs for each parsed role, keyed on the `Role.ID`
// - Directives: a map of `Directive` structs for each parsed directive, keyed on the `Directive.ID`
type Inventory struct {
	inventoryPath string
	Hosts         map[string]Host
	Modules       map[string]Module
	Roles         map[string]Role
	Directives    []DirectiveScript
}

// NewInventory parses the files/directories in the provided path
// to populate the inventory.
func NewInventory(path string) Inventory {
	i := Inventory{
		inventoryPath: path,
		Hosts:         make(map[string]Host),
		Modules:       make(map[string]Module),
		Roles:         make(map[string]Role),
		Directives:    []DirectiveScript{},
	}

	return i
}

// Reload reloads Inventory from it's configured path. Components that are reloaded:
// - Hosts
// - Roles
// - Modules
// - Directives
func (i *Inventory) Reload() {
	// populate the inventory

	// parse hosts
	if err := i.ParseHosts(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload hosts")
	}

	// parse roles
	if err := i.ParseRoles(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload roles")
	}

	// parse modules
	if err := i.ParseModules(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload modules")
	}

	// parse directives
	if err := i.ParseDirectives(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload directives")
	}
}

// IsEnrolled returns true is this system's hostname is found
// in the inventory's Host map, and false otherwise.
func (i *Inventory) IsEnrolled() bool {
	_, found := i.Hosts[self.GetHostname()]
	return found
}

// InitInventory should be called during service startup/initialization to create the
// globalInventory that is used internally.
func InitInventory() {
	// on first load, do an initial search for all mangos in specified path
	once.Do(func() {
		inventoryPath := viper.GetString("mango.inventory")

		globalInventory := NewInventory(inventoryPath)
		globalInventory.Reload()
	})
}
