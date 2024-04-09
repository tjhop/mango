package version

import (
	"fmt"
	"runtime"
)

var (
	Version   string // will be populated by linker during `go build`
	BuildDate string // will be populated by linker during `go build`
	Commit    string // will be populated by linker during `go build`
)

// Print outputs human readable build about the binary to stdout
// Models return on: github.com/prometheus/common/version.Print()
func Print(programName string) string {
	return fmt.Sprintf("%s build info:\n\tversion: %s\n\tbuild date: %s\n\tcommit: %s\n\tgo version: %s\n",
		programName,
		Version,
		BuildDate,
		Commit,
		runtime.Version(),
	)
}
