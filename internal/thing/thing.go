package thing

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

var (
	// prometheus metrics
	// exported so that it can be set by each package that implements the Thing interface
	MetricThingsLastRunTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_thing_last_run_seconds",
			Help: "Timestamp of the last Run the labeled thing has performed, in seconds since the epoch",
		},
		[]string{"type", "id"},
	)

	MetricRunCountTotal = prometheus.NewCounterVec(
		prometheus.GaugeOpts{
			Name: "mango_thing_run_count_total",
			Help: "A count of the total number of runs that have been performed to manage the labeled thing",
		},
		[]string{"type", "id", "result"},
	)

	MetricCheckCountTotal = prometheus.NewCounterVec(
		prometheus.GaugeOpts{
			Name: "mango_thing_check_count_total",
			Help: "A count of the total number of checks that have been performed to determine if the labeled thing is in the desired state",
		},
		[]string{"type", "id"},
	)
)

// RunStat tracks various runtime information about the Thing.
// - LastRunTimestamp: timestamp of the last time a run was started for this Thing
// - LastSuccessTimestamp: timestamp of the last successful run for this Thing
// - RunCount: how many runs this Thing has performed to update the resource backing the Thing
// - CheckCount: how many times this Thing performed idempotency checks on the resource backing the Thing
type RunStat struct {
	LastRunTimestamp     time.Time
	LastSuccessTimestamp time.Time
	RunCount             int
	CheckCount           int
}

// BaseThing provides a base set of attributes that all Thing types should include in their package-specific
// structs to implement the Thing interface for management.
// - RunStats: a RunStat struct to track runtime statistics for the thing
// - Logger: a base logrus.Entry object for context specific logging
// - ID: a string representing the ID for this thing, to be parsed from the mango config . Intended to be
//   used for dependency tracking.
// - Type: a string representing the type of thing being managed
type BaseThing struct {
	RunStats RunStat
	Logger   log.Entry
	ID       string
	Type     string
}

func (t *BaseThing) String() string { return t.ID }

func (t *BaseThing) Manage() error { return nil }

// NewThing returns an ID'd/type'd BaseThing, suitable for use initializers for future Thing types
func NewThing(id, thingType string) BaseThing {
	t := BaseThing{
		RunStats: Runstat{},
		ID: id,
		Type: thingType,
		Logger: log.WithFields(log.Fields{
			"thing": thingType,
			"id": id,
		})
	}

	return t
}
