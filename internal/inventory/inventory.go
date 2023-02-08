package inventory

import (
	"context"
	"path/filepath"

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
	variables     VariableMap
}

// String is a stringer to return the inventory path
func (i *Inventory) String() string { return i.inventoryPath }

// Store is the set of methods that Inventory must
// implement to serve as a backing store for an inventory
// implementation. This is to try and keep a consistent API
// in the event that other inventory types are introduced,
// as well as to keep the required methods centralized if new
// features are introduced.
type Store interface {
	// Inventory management functions
	Reload(ctx context.Context)

	// Enrollment Checks
	IsEnrolled() bool

	// General Inventory Getters
	GetDirectives() []DirectiveScript
	GetHosts() []Host
	GetModules() []Module
	GetRoles() []Role

	// Inventory checks by component IDs
	GetHost(host string) (Host, bool)
	GetModule(module string) (Module, bool)
	GetRole(role string) (Role, bool)

	// Checks by host
	GetDirectivesForHost(host string) []DirectiveScript
	GetModulesForRole(role string) []Module
	GetModulesForHost(host string) []Module
	GetRolesForHost(host string) []Role
	GetVariablesForHost(host string) VariableMap

	// Self checks
	GetDirectivesForSelf() []DirectiveScript
	GetModulesForSelf() []Module
	GetRolesForSelf() []Role
	GetVariablesForSelf() VariableMap
}

// NewInventory parses the files/directories in the provided path
// to populate the inventory.
func NewInventory(path string) *Inventory {
	i := Inventory{
		inventoryPath: path,
		hosts:         []Host{},
		modules:       []Module{},
		roles:         []Role{},
		directives:    []DirectiveScript{},
		variables:     make(VariableMap),
	}

	return &i
}

// Reload reloads Inventory from it's configured path. Components that are reloaded:
// - Hosts
// - Roles
// - Modules
// - Directives
func (i *Inventory) Reload(ctx context.Context) {
	// populate the inventory

	// parse hosts
	if err := i.ParseHosts(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload hosts")
	}

	// parse roles
	if err := i.ParseRoles(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload roles")
	}

	// parse modules
	if err := i.ParseModules(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload modules")
	}

	// parse directives
	if err := i.ParseDirectives(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to reload directives")
	}

	// parse global variables
	globalVars := filepath.Join(i.inventoryPath, "variables")
	vars, err := ParseVariables(ctx, globalVars)
	if err != nil {
		log.WithFields(log.Fields{
			"file":  globalVars,
			"error": err,
		}).Error("Failed to parse global variables")
	}

	i.variables = vars
}

// IsEnrolled returns true is this system's hostname is found
// in the inventory's Host map, and false otherwise.
func (i *Inventory) IsEnrolled() bool {
	me := self.GetHostname()
	_, found := i.GetHost(me)
	return found
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

// GetModule returns a copy of the Module struct for a module identified by
// `module`, and a boolean indicating whether or not the named module was found
// in the inventory.
func (i *Inventory) GetModule(module string) (Module, bool) {
	for _, m := range i.modules {
		if filepath.Base(m.id) == module {
			return m, true
		}
	}

	return Module{}, false
}

// GetModules returns a copy of the inventory's Modules.
func (i *Inventory) GetModules() []Module {
	return i.modules
}

// GetModulesForRole returns a slice of Modules, containing
// all of the Modules for the specified role.
func (i *Inventory) GetModulesForRole(role string) []Module {
	mods := []Module{}
	if r, found := i.GetRole(role); found {
		for _, m := range r.modules {
			if roleMods, found := i.GetModule(m); found {
				mods = append(mods, roleMods)
			}
		}
	}

	return filterDuplicateModules(mods)
}

// GetModulesForHost returns a slice of Modules, containing all of the
// Modules for the specified host system (including modules in all assigned roles, as well as ad-hoc modules).
func (i *Inventory) GetModulesForHost(host string) []Module {
	mods := []Module{}

	if h, found := i.GetHost(host); found {
		// get modules from all roles host is assigned
		for _, r := range h.roles {
			mods = append(mods, i.GetModulesForRole(r)...)
		}

		// get raw host modules
		for _, m := range h.modules {
			if mod, found := i.GetModule(m); found {
				mods = append(mods, mod)
			}
		}
	}

	return filterDuplicateModules(mods)
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
func (i *Inventory) GetRole(role string) (Role, bool) {
	for _, r := range i.roles {
		if filepath.Base(r.id) == role {
			return r, true
		}
	}

	return Role{}, false
}

// GetRoles returns a copy of the inventory's Roles.
func (i *Inventory) GetRoles() []Role {
	return i.roles
}

// GetRolesForHost returns a slice of Roles, containing all of the
// Roles for the specified host system.
func (i *Inventory) GetRolesForHost(host string) []Role {
	if h, found := i.GetHost(host); found {
		roles := []Role{}
		for _, r := range h.roles {
			if role, found := i.GetRole(r); found {
				roles = append(roles, role)
			}
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

// GetHost returns a copy of the Host struct for a system identified by `host`
// name, and a boolean indicating whether or not the named host was found in
// the inventory.
func (i *Inventory) GetHost(host string) (Host, bool) {
	for _, h := range i.hosts {
		if filepath.Base(h.id) == host {
			return h, true
		}
	}

	return Host{}, false
}

// GetVariablesForHost returns a copy of the variable map for the system
// `host`, or nil if the host was not found.
func (i *Inventory) GetVariablesForHost(host string) VariableMap {
	// TODO: eventually, merging variables need to be supported. override
	// over should be:
	// - global vars
	// - module vars
	// - host vars
	if h, found := i.GetHost(host); found {
		return h.variables
	}

	return nil
}

// GetVariablesForSelf returns a copy of the variable map for the running
// system.
func (i *Inventory) GetVariablesForSelf() VariableMap {
	me := self.GetHostname()
	return i.GetVariablesForHost(me)
}

func filterDuplicateModules(input []Module) []Module {
	modMap := make(map[string]Module)

	for _, m := range input {
		if _, found := modMap[m.String()]; !found {
			modMap[m.String()] = m
		}
	}

	output := make([]Module, len(modMap))
	for _, mod := range modMap {
		output = append(output, mod)
	}

	return output
}
