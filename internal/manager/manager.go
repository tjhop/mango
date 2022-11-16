package manager

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/inventory"
)

// Manager contains fields related to track and execute runnable modules and statistics.
type Manager struct {
	id         string
	logger     *log.Entry
	modules    []inventory.Module
	directives []inventory.DirectiveScript
}

func (m *Manager) String() string { return m.id }

// NewManager returns a new Manager struct instantiated with the given ID
func NewManager(id string) *Manager {
	return &Manager{
		id: id,
		logger: log.WithFields(log.Fields{
			"manager": id,
		}),
	}
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (m *Manager) Reload(inv inventory.Store) {
	m.logger.Info("Reloading items from inventory")

	modules := inv.GetModulesForSelf()
	m.logger.WithFields(log.Fields{
		"old": m.modules,
		"new": modules,
	}).Debug("Reloading modules from inventory")
	m.modules = modules

	directives := inv.GetDirectivesForSelf()
	m.logger.WithFields(log.Fields{
		"old": m.directives,
		"new": directives,
	}).Debug("Reloading directives from inventory")
	m.directives = directives
}

// RunDirectives runs all of the directive scripts being managed by the Manager
func (m *Manager) RunDirectives(ctx context.Context) {
	if len(m.directives) <= 0 {
		m.logger.Info("No Directives to run")
		return
	}

	for _, d := range m.directives {
		m.logger.WithFields(log.Fields{
			"path": d,
		}).Info("Running directive")

		if err := d.Run(ctx); err != nil {
			m.logger.WithFields(log.Fields{
				"path": d,
			}).Error("Directive failed")
		}
	}
}

// RunModules runs all of the modules being managed by the Manager
func (m *Manager) RunModules(ctx context.Context) {
	if len(m.modules) <= 0 {
		m.logger.Info("No Modules to run")
		return
	}

	for _, mod := range m.modules {
		m.logger.WithFields(log.Fields{
			"path": mod,
		}).Info("Running Module")

		if err := mod.Run(ctx); err != nil {
			m.logger.WithFields(log.Fields{
				"path": mod,
			}).Error("Module failed")
		}
	}
}

// RunAll runs all of the Directives being managed by the Manager, followed by
// all of the Modules being managed by the Manager.
func (m *Manager) RunAll(ctx context.Context) {
	m.RunDirectives(ctx)
	m.RunModules(ctx)
}
