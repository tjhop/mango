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
// - Hosts: a slice of `Host` structs for each parsed host
// - Modules: a slice of `Module` structs for each parsed module
// - Roles: a slice of `Role` structs for each parsed role
// - Directives: a slice of `DirectiveScript` structs, containing for each parsed directive
type Inventory struct {
	inventoryPath string
	Hosts         []Host
	Modules       []Module
	Roles         []Role
	Directives    []DirectiveScript
}

// NewInventory parses the files/directories in the provided path
// to populate the inventory.
func NewInventory(path string) Inventory {
	i := Inventory{
		inventoryPath: path,
		Hosts:         []Host{},
		Modules:       []Module{},
		Roles:         []Role{},
		Directives:    []DirectiveScript{},
	}

	return i
}

// Reload calls `inventory.Reload()` against the global inventory.
// Useful for other packages (like managers) to trigger inventory reloads.
func Reload() {
	globalInventory.Reload()
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

// IsHostEnrolled calls the `IsHostEnrolled()` method against the globalInventory.
// Should be used by other packages to verify host system enrollment during runtime.
func IsHostEnrolled(host string) bool {
	return globalInventory.IsHostEnrolled(host)
}

// IsHostEnrolled returns true if the named host is found
// within the inventory's Host list.
func (i *Inventory) IsHostEnrolled(host string) bool {
	for _, h := range i.Hosts {
		if h.ID == host {
			return true
		}
	}

	return false
}

// IsEnrolled calls the `IsEnrolled()` method against the globalInventory.
// Should be used by other packages to verify host system enrollment during runtime.
func IsEnrolled() bool {
	return globalInventory.IsEnrolled()
}

// IsEnrolled returns true is this system's hostname is found
// in the inventory's Host map, and false otherwise.
func (i *Inventory) IsEnrolled() bool {
	me := self.GetHostname()
	return i.IsHostEnrolled(me)
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
