package inventory

import (
	"time"
)

// RunStat tracks various runtime information about the script.
// - LastRunTimestamp: timestamp of the last time a run was started for this script
// - LastSuccessTimestamp: timestamp of the last successful run for this script
// - RunCount: how many runs this script has performed to update the resource backing the script
// - CheckCount: how many times this script performed idempotency checks on the resource backing the script
type RunStat struct {
	ApplyExitCode        int
	TestExitCode         int
	LastRunTimestamp     time.Time
	LastSuccessTimestamp time.Time
	ApplyCount           int
	TestCount            int
}

// Script contains fields that are relevant to all of the executable scripts mango will be working with.
// - ID: string identifying the script (the name of the script)
// - Path: Absolute path to the script
type Script struct {
	ID    string
	Path  string
	Stats RunStat
}
