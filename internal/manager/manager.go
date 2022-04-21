package manager

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/thing"
)

// Thing interface is the set of methods that any specific thing type (file, systemd, etc) must
// implement in order to be a "thing" that can be managed.
// The `Manage()` method should be a goroutine safe, idempotent function that gets the desired thing
// into the correct state, as defined by the mango configuration files. It returns an error if
// one is received, and nil otherwise.
// The `String()` method is just a standard stringer interface.
type Thing interface {
	Manage() error
	String() string
}

// BaseManager provides a base set of attributes that all Manager types should include in their package-specific
// structs to implement the Manager interface.
// - Logger: a base logrus.Entry object for context specific logging
// - ID: a string representing the ID for this manager, will likely be the manager type with some form of unique identifier
// - Things: a slice of objects that fulfill the Thing interface for management
type BaseManager struct {
	Logger log.Entry
	ID     string
	Things map[string]thing.Thing
}

func (m *BaseManager) String() string { return m.ID }

// NewManager returns a ID'd BaseManager, suitable for use initializers for future Manager types
func NewManager(id string) BaseManager {
	m := BaseManager {
		ID: id,
		Logger: log.WithFields(log.Fields{
			"id": id,
		}),
		Things: map[string]thing.Thing
	}

	return m
}
