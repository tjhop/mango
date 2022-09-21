package inventory

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/self"
)

var (
	commonMetricLabels = []string{"inventory", "component", "hostname"}

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
	hosts         []Host
	modules       []Module
	roles         []Role
	directives    []DirectiveScript
}

// Store is the set of methods that Inventory must
// implement to serve as a backing store for an inventory
// implementation. This is to try and keep a consistent API
// in the event that other inventory types are introduced,
// as well as to keep the required methods centralized if new
// features are introduced.
type Store interface {
	// Inventory management functions
	Reload()

	// Enrollment Checks
	IsHostEnrolled(host string) bool
	IsEnrolled() bool

	// General Inventory Getters
	GetDirectives() []DirectiveScript
	GetHosts() []Host
	GetModules() []Module
	GetRoles() []Role

	// Inventory checks by component IDs
	GetDirectivesForHost(host string) []DirectiveScript
	GetHost(host string) Host
	GetModule(module string) Module
	GetModulesForHost(host string) []Module
	GetRole(role string) Role
	GetRolesForHost(host string) []Role

	// Self checks
	GetDirectivesForSelf() []DirectiveScript
	GetModulesForSelf() []Module
	GetRolesForSelf() []Role
}

// NewInventory parses the files/directories in the provided path
// to populate the inventory.
func NewInventory(path string) Inventory {
	i := Inventory{
		inventoryPath: path,
		hosts:         []Host{},
		modules:       []Module{},
		roles:         []Role{},
		directives:    []DirectiveScript{},
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

// IsHostEnrolled returns true if the named host is found
// within the inventory's Host list.
func (i *Inventory) IsHostEnrolled(host string) bool {
	for _, h := range i.hosts {
		if h.id == host {
			return true
		}
	}

	return false
}

// IsEnrolled returns true is this system's hostname is found
// in the inventory's Host map, and false otherwise.
func (i *Inventory) IsEnrolled() bool {
	me := self.GetHostname()
	return i.IsHostEnrolled(me)
}

// GetDirectives returns a copy of the inventory's slice of DirectiveScript
func (i *Inventory) GetDirectives() []DirectiveScript {
	return i.directives
}

// GetDirectivesForHost returns a copy of the inventory's slice of DirectiveScript.
// Since directives are applied to all hosts, this internally just calls
// `inventory.GetDirectives()`
func (i *Inventory) GetDirectivesForHost(host string) []DirectiveScript {
	return i.GetDirectives()
}

// GetDirectivesForSelf returns a copy of the inventory's slice of DirectiveScript.
// Since directives are applied to all hosts, this internally just calls
// `inventory.GetDirectives()`
func (i *Inventory) GetDirectivesForSelf() []DirectiveScript {
	return i.GetDirectives()
}

// GetModule returns a copy of the Module struct for a module identified
// by `module`. If the named module is not found in the inventory, an
// empty Module is returned.
func (i *Inventory) GetModule(module string) Module {
	for _, m := range i.modules {
		if m.id == module {
			return m
		}
	}

	return Module{}
}

// GetModules returns a copy of the inventory's Modules.
func (i *Inventory) GetModules() []Module {
	return i.modules
}

// GetModulesForHost returns a slice of Modules, containing all of the
// Modules for the specified host system.
func (i *Inventory) GetModulesForHost(host string) []Module {
	if i.IsHostEnrolled(host) {
		mods := []Module{}
		h := i.GetHost(host)

		// get raw host modules
		for _, m := range h.modules {
			mods = append(mods, i.GetModule(m))
		}

		return mods
	}

	return nil
}

// GetModulesForSelf returns a slice of Modules, containing all of the
// Modules for the running system from the inventory.
func (i *Inventory) GetModulesForSelf() []Module {
	me := self.GetHostname()
	return i.GetModulesForHost(me)
}

// GetRole returns a copy of the Role struct for a role identified
// by `role`. If the named role is not found in the inventory, an
// empty Role is returned.
func (i *Inventory) GetRole(role string) Role {
	for _, r := range i.roles {
		if r.id == role {
			return r
		}
	}

	return Role{}
}

// GetRoles returns a copy of the inventory's Roles.
func (i *Inventory) GetRoles() []Role {
	return i.roles
}

// GetRolesForHost returns a slice of Roles, containing all of the
// Roles for the specified host system.
func (i *Inventory) GetRolesForHost(host string) []Role {
	if i.IsHostEnrolled(host) {
		roles := []Role{}
		h := i.GetHost(host)
		for _, r := range h.roles {
			roles = append(roles, i.GetRole(r))
		}

		return roles
	}

	return nil
}

// GetRolesForSelf returns a slice of Roles, containing all of the
// Roles for the running system from the inventory.
func (i *Inventory) GetRolesForSelf() []Role {
	me := self.GetHostname()
	return i.GetRolesForHost(me)
}

// GetHosts returns a copy of the inventory's Hosts.
func (i *Inventory) GetHosts() []Host {
	return i.hosts
}

// GetHost returns a copy of the Host struct for a system
// identified by `host` name. If the hostname is not found
// in the inventory, an empty Host is returned.
func (i *Inventory) GetHost(host string) Host {
	for _, h := range i.hosts {
		if h.id == host {
			return h
		}
	}

	return Host{}
}
