package inventory

import (
	"context"
	"log/slog"
	"path/filepath"
	"strconv"

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

// Inventory contains fields that comprise the data that makes up our inventory.
// - Hosts: a slice of `Host` structs for each parsed host
// - Modules: a slice of `Module` structs for each parsed module
// - Roles: a slice of `Role` structs for each parsed role
// - Directives: a slice of `Directive` structs, containing for each parsed directive
// - Groups: a slice of `Group` structs, containing globs/patterns for hostname matching
type Inventory struct {
	inventoryPath string
	hostname      string
	hosts         []Host
	modules       []Module
	roles         []Role
	directives    []Directive
	groups        []Group
}

// String is a stringer to return the inventory path
func (i *Inventory) String() string { return i.inventoryPath }

// GetInventoryPath returns the inventory path as a string
func (i *Inventory) GetInventoryPath() string { return i.inventoryPath }

// GetHostname returns the inventory path as a string
func (i *Inventory) GetHostname() string { return i.hostname }

// Store is the set of methods that Inventory must
// implement to serve as a backing store for an inventory
// implementation. This is to try and keep a consistent API
// in the event that other inventory types are introduced,
// as well as to keep the required methods centralized if new
// features are introduced.
type Store interface {
	// Inventory management functions
	Reload(ctx context.Context, logger *slog.Logger)

	// Enrollment and runtime/metadata checks
	IsEnrolled() bool
	GetInventoryPath() string
	GetHostname() string

	// General Inventory Getters
	GetDirectives() []Directive
	GetHosts() []Host
	GetModules() []Module
	GetRoles() []Role
	GetGroups() []Group

	// Inventory checks by component IDs
	GetHost(host string) (Host, bool)
	GetModule(module string) (Module, bool)
	GetRole(role string) (Role, bool)
	GetGroup(group string) (Group, bool)

	// Checks by host
	GetDirectivesForHost(host string) []Directive
	GetModulesForRole(role string) []Module
	GetModulesForHost(host string) []Module
	GetRolesForHost(host string) []Role
	GetVariablesForHost(host string) string

	// Self checks
	GetDirectivesForSelf() []Directive
	GetModulesForSelf() []Module
	GetRolesForSelf() []Role
	GetVariablesForSelf() []string
}

// NewInventory parses the files/directories in the provided path
// to populate the inventory.
func NewInventory(path, name string) *Inventory {
	i := Inventory{
		inventoryPath: path,
		hostname:      name,
		hosts:         []Host{},
		modules:       []Module{},
		roles:         []Role{},
		directives:    []Directive{},
	}
	metricMangoInventoryInfoLabels["hostname"] = name
	metricMangoInventoryInfoLabels["inventory_path"] = path
	metricMangoInventoryInfo.With(metricMangoInventoryInfoLabels).Set(1)

	return &i
}

// Reload reloads Inventory from it's configured path. Components that are reloaded:
// - Hosts
// - Roles
// - Modules
// - Directives
func (i *Inventory) Reload(ctx context.Context, logger *slog.Logger) {
	// populate the inventory

	// parse groups
	if err := i.ParseGroups(ctx, logger); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to reload groups",
			slog.String("err", err.Error()),
		)
	}

	// parse hosts
	if err := i.ParseHosts(ctx, logger); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to reload hosts",
			slog.String("err", err.Error()),
		)
	}

	// parse roles
	if err := i.ParseRoles(ctx, logger); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to reload roles",
			slog.String("err", err.Error()),
		)
	}

	// parse modules
	if err := i.ParseModules(ctx, logger); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to reload modules",
			slog.String("err", err.Error()),
		)
	}

	// parse directives
	if err := i.ParseDirectives(ctx, logger); err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to reload directives",
			slog.String("err", err.Error()),
		)
	}

	// update inventory metrics
	metricMangoInventoryInfoLabels["enrolled"] = strconv.FormatBool(i.IsEnrolled())
	metricMangoInventoryInfo.With(metricMangoInventoryInfoLabels).Set(1)
}

// IsEnrolled returns if the hostname of the system is defined in the
// inventory, or if the hostname of the system matches any group match
// parameters
func (i *Inventory) IsEnrolled() bool {
	if _, found := i.GetHost(i.hostname); found {
		return true
	}

	for _, group := range i.groups {
		if group.IsHostEnrolled(i.hostname) {
			return true
		}
	}

	return false
}

// GetDirectives returns a copy of the inventory's slice of Directive
func (i *Inventory) GetDirectives() []Directive {
	return i.directives
}

// GetDirectivesForHost returns a copy of the inventory's slice of Directive.
// Since directives are applied to all hosts, this internally just calls
// `inventory.GetDirectives()`
func (i *Inventory) GetDirectivesForHost(host string) []Directive {
	return i.GetDirectives()
}

// GetDirectivesForSelf returns a copy of the inventory's slice of Directive.
// Since directives are applied to all hosts, this internally just calls
// `inventory.GetDirectives()`
func (i *Inventory) GetDirectivesForSelf() []Directive {
	return i.GetDirectives()
}

// GetModule returns a copy of the Module struct for a module identified by
// `module`, and a boolean indicating whether or not the named module was found
// in the inventory.
func (i *Inventory) GetModule(module string) (Module, bool) {
	for _, m := range i.modules {
		if filepath.Base(m.ID) == module {
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
			if roleMod, found := i.GetModule(m); found {
				mods = append(mods, roleMod)
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

// GetModulesForGroup returns a slice of Modules, containing all of the
// Modules for the specified host system (including modules in all assigned roles, as well as ad-hoc modules).
func (i *Inventory) GetModulesForGroup(group string) []Module {
	mods := []Module{}

	if g, found := i.GetGroup(group); found {
		// get modules from all roles group is assigned
		for _, r := range g.roles {
			mods = append(mods, i.GetModulesForRole(r)...)
		}

		// get raw group modules
		for _, m := range g.modules {
			if mod, found := i.GetModule(m); found {
				mods = append(mods, mod)
			}
		}
	}

	return filterDuplicateModules(mods)
}

func (i *Inventory) GetRolesForGroup(group string) []Role {
	roles := []Role{}

	if g, found := i.GetGroup(group); found {
		for _, r := range g.roles {
			if role, found := i.GetRole(r); found {
				roles = append(roles, role)
			}
		}
	}
	return roles
}

// GetModulesForSelf returns a slice of Modules, containing all of the
// Modules for the running system from the inventory.
func (i *Inventory) GetModulesForSelf() []Module {
	var mods []Module

	mods = append(mods, i.GetModulesForHost(i.hostname)...)
	for _, group := range i.groups {
		mods = append(mods, i.GetModulesForGroup(group.String())...)
	}

	return filterDuplicateModules(mods)
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
	var roles []Role

	roles = append(roles, i.GetRolesForHost(i.hostname)...)
	for _, group := range i.groups {
		roles = append(roles, i.GetRolesForGroup(group.String())...)
	}

	return filterDuplicateRoles(roles)
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

// GetVariablesForHost returns the path of the host's variables file, or the
// empty string if no host/variables file found
func (i *Inventory) GetVariablesForHost(host string) string {
	if h, found := i.GetHost(host); found {
		return h.variables
	}

	return ""
}

// GetVariablesForSelf returns slice of strings, containing the paths of any
// variables files found for this host. All group variables a provided first,
// with host-specific variables provided last (to allow for overriding default
// group variable data).
func (i *Inventory) GetVariablesForSelf() []string {
	var tmp, varFiles []string

	for _, group := range i.groups {
		if group.IsHostEnrolled(i.hostname) {
			tmp = append(tmp, group.variables)
		}
	}

	tmp = append(tmp, i.GetVariablesForHost(i.hostname))
	for _, file := range tmp {
		if file != "" {
			varFiles = append(varFiles, file)
		}
	}

	return varFiles
}

func filterDuplicateModules(input []Module) []Module {
	modMap := make(map[string]Module)

	for _, m := range input {
		if _, found := modMap[m.String()]; !found {
			modMap[m.String()] = m
		}
	}

	var output []Module
	for _, mod := range modMap {
		output = append(output, mod)
	}

	return output
}

func filterDuplicateRoles(input []Role) []Role {
	roleMap := make(map[string]Role)

	for _, r := range input {
		if _, found := roleMap[r.String()]; !found {
			roleMap[r.String()] = r
		}
	}

	var output []Role
	for _, role := range roleMap {
		output = append(output, role)
	}

	return output
}

// GetGroups returns a copy of the inventory's Groups.
func (i *Inventory) GetGroups() []Group {
	return i.groups
}

// GetGroup returns a copy of the Group struct for a system identified by `group`
// name, and a boolean indicating whether or not the named group was found in
// the inventory.
func (i *Inventory) GetGroup(group string) (Group, bool) {
	for _, g := range i.groups {
		if filepath.Base(g.id) == group {
			return g, true
		}
	}

	return Group{}, false
}

// GetVariablesForGroup returns the path of the group's variables file, or the
// empty string if no group/variables file found
func (i *Inventory) GetVariablesForGroup(group string) string {
	if g, found := i.GetGroup(group); found {
		return g.variables
	}

	return ""
}
