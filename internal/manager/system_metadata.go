package manager

import (
	"context"
	"fmt"
	"os"
	"runtime"

	kernelParser "github.com/moby/moby/pkg/parsers/kernel"
	distro "github.com/quay/claircore/osrelease"
	log "github.com/sirupsen/logrus"
)

func getOSMetadata() osMetadata {
	// os metadata for templates
	osReleaseFile, err := os.Open(distro.Path)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  distro.Path,
		}).Error("Failed to open os-release file")
	}
	osRelease, err := distro.Parse(context.Background(), osReleaseFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  distro.Path,
		}).Error("Failed to parse os-release file")
	}
	osData := osMetadata{
		OSRelease: osRelease,
	}

	return osData
}

func getKernelMetadata() kernelMetadata {
	// kernel metadata for templates
	kernelInfo, err := kernelParser.GetKernelVersion()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to parse kernel info")
	}
	kernelData := kernelMetadata{
		Kernel: kernelInfo.Kernel,
		Major:  kernelInfo.Major,
		Minor:  kernelInfo.Minor,
		Flavor: kernelInfo.Flavor,
		Full:   fmt.Sprintf("%d.%d.%d%s", kernelInfo.Kernel, kernelInfo.Major, kernelInfo.Minor, kernelInfo.Flavor),
	}

	return kernelData
}

func getCPUMetadata() cpuMetadata {
	return cpuMetadata{
		NumCPU: runtime.NumCPU(),
	}
}
