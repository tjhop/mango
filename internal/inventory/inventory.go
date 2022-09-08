package inventory

import (
	"errors"
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

	// custom errors

	// ErrHostNotEnrolled is returned when a host isn't enrolled
	ErrHostNotEnrolled = errors.New("Host not enrolled in inventory")

	// prometheus metrics
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

// GetDirectives returns a copy of the global inventory's slice of `DirectiveScript`'s
// Internally, it calls the `GetDirectives()` method against the global inventory.
// Should be used by other packages.
func GetDirectives() ([]DirectiveScript) {
	return globalInventory.Directives
}

// GetDirectives returns a copy of the inventory's slice of `DirectiveScript`'s
func (i *Inventory) GetDirectives() ([]DirectiveScript) {
	return i.Directives
}

// GetModules returns a copy of the global inventory's Modules.
// Internally, it calls the `GetModules()` method against the global inventory.
// Should be used by other packages.
func GetModules() []Module {
	return globalInventory.Modules
}

// GetModules returns a copy of the inventory's Modules.
func (i *Inventory) GetModules() []Module {
	return i.Modules
}

// GetModule returns a copy of the Module struct for a module identified
// by `module`. If the named module is not found in the inventory, an
// empty Module is returned.
func (i *Inventory) GetModule(module string) Module {
	for _, m := range i.Modules {
		if m.ID == module {
			return m
		}
	}

	return Module{}
}

// GetModule returns a copy of the Module struct for a module identified
// by `module` in the global inventory. If the named module is not
// found in the inventory, an empty Module is returned.
func GetModule(module string) Module {
	return globalInventory.GetModule(module)
}

// GetRoles returns a copy of the global inventory's Roles.
// Internally, it calls the `GetRoles()` method against the global inventory.
// Should be used by other packages.
func GetRoles() []Role {
	return globalInventory.Roles
}

// GetRoles returns a copy of the inventory's Roles.
func (i *Inventory) GetRoles() []Role {
	return i.Roles
}

// GetRole returns a copy of the Role struct for a role identified
// by `role`. If the named role is not found in the inventory, an
// empty Role is returned.
func (i *Inventory) GetRole(role string) Role {
	for _, r := range i.Roles {
		if r.ID == role {
			return r
		}
	}

	return Role{}
}

// GetRole returns a copy of the Role struct for a role identified
// by `role` in the global inventory. If the named role is not
// found in the inventory, an empty Role is returned.
func GetRole(role string) Role {
	return globalInventory.GetRole(role)
}

// GetHosts returns a copy of the global inventory's Hosts map of Host ID -> Host struct.
// Internally, it calls the `GetHosts()` method against the global inventory.
// Should be used by other packages.
func GetHosts() []Host {
	return globalInventory.Hosts
}

// GetHosts returns a copy of the inventory's Hosts map of Host ID -> Host struct.
func (i *Inventory) GetHosts() []Host {
	return i.Hosts
}

// GetHost returns a copy of the Host struct for a system
// identified by `host` name. If the hostname is not found
// in the inventory, an empty Host is returned.
func (i *Inventory) GetHost(host string) Host {
	for _, h := range i.Hosts {
		if h.ID == host {
			return h
		}
	}

	return Host{}
}

// GetHost returns a copy of the Host struct for a system
// identified by `host` name in the global inventory. If the
// hostname is not found in the inventory, an empty Host is returned.
func GetHost(host string) Host {
	return globalInventory.GetHost(host)
}

// GetModulesForHost returns a slice of Modules, containing all of the
// Modules for the specified host system.
func (i *Inventory) GetModulesForHost(host string) ([]Module, error) {
	if i.IsHostEnrolled(host) {
		h := i.GetHost(host)
		mods := h.GetModules()

		return mods, nil
	}

	return nil, ErrHostNotEnrolled
}

// GetModulesForHost returns a slice of Modules, containing all of the
// Modules for the specified host system from the global inventory.
func GetModulesForHost(host string) ([]Module, error) {
	if IsHostEnrolled(host) {
		h := GetHost(host)
		mods := h.GetModules()

		return mods, nil
	}

	return nil, ErrHostNotEnrolled
}

// GetModulesForSelf returns a slice of Modules, containing all of the
// Modules for the running system from the global inventory.
func GetModulesForSelf() ([]Module, error) {
	me := self.GetHostname()
	return GetModulesForHost(me)
}

// GetRolesForHost returns a slice of Roles, containing all of the
// Roles for the specified host system.
func (i *Inventory) GetRolesForHost(host string) ([]Role, error) {
	if i.IsHostEnrolled(host) {
		h := i.GetHost(host)
		roles := h.GetRoles()

		return roles, nil
	}

	return nil, ErrHostNotEnrolled
}

// GetRolesForHost returns a slice of Roles, containing all of the
// Roles for the specified host system from the global inventory.
func GetRolesForHost(host string) ([]Role, error) {
	if IsHostEnrolled(host) {
		h := GetHost(host)
		roles := h.GetRoles()

		return roles, nil
	}

	return nil, ErrHostNotEnrolled
}

// GetRolesForSelf returns a slice of Roles, containing all of the
// Roles for the running system from the global inventory.
func GetRolesForSelf() ([]Role, error) {
	me := self.GetHostname()
	return GetRolesForHost(me)
}

// InitInventory should be called during service startup/initialization to create the
// globalInventory that is used internally.
func InitInventory() {
	// on first load, do an initial search for all mangos in specified path
	once.Do(func() {
		inventoryPath := viper.GetString("inventory.path")

		globalInventory := NewInventory(inventoryPath)
		globalInventory.Reload()
	})
}
