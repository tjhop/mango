package manager

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/inventory"
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

// RunDirectives runs all of the directive scripts being managed by the Manager
func (m *Manager) RunDirectives(ctx context.Context) {
	for _, d := range m.directives {
		log.WithFields(log.Fields{
			"path": d,
		}).Info("Running directive")

		if err := d.Run(ctx); err != nil {
			log.WithFields(log.Fields{
				"path": d,
			}).Error("Directive failed")
		}
	}
}

// RunModules runs all of the modules being managed by the Manager
func (m *Manager) RunModules(ctx context.Context) {
	for _, d := range m.modules {
		log.WithFields(log.Fields{
			"path": d,
		}).Info("Running module")

		if err := d.Run(ctx); err != nil {
			log.WithFields(log.Fields{
				"path": d,
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
