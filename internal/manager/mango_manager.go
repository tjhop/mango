package manager

import (
	"sync"
)

const (
	mangoManagerID = "Mango Manager"
)

var (
	globalMangoManager MangoManager
)

// Manager interface is the set of methods that any specific manager type (file, systemd, etc) must ipmlement
// in order to be registerd with and managed by the main Mango manager.
// - ManageAll should call the Manage() method for each thing it is managing
// - RefreshAll should have the Manager refresh it's list of things it is managing.
type Manager interface {
	ManageAll() error
	RefreshAll() error
	String() string
}

// MangoManager is a special manager, who's job is to manage other managers that fulfull the Manager interface
type MangoManager struct {
	ID       string
	Logger   log.Entry
	managers map[string]Manager
}

func (mm *MangoManager) String() string { return mm.ID }

// NewMangoManager returns an initialized mango manager. Since this manager is designed to manage other managers,
// it's expected that there be only one.
func NewMangoManager() MangoManager {
	mm := MangoManager{
		ID: mangoManagerID,
		Logger: log.WithFields(log.Fields{
			"id": mangoManagerID,
		}),
		managers: make(map[string]Manager),
	}

	return mm
}

func init() {
	once.Do(func() {
		globalMangoManager = NewMangoManager()
	})
}

// Register takes an object that satisfies the Manager interface and adds it to the map of Managers that the Mango Manager will manage (boy, what a mouthful)
func Register(m Manager) {
	globalMangoManager[m] = m
}
