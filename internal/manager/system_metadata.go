package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	kernelParser "github.com/moby/moby/pkg/parsers/kernel"
	"github.com/pbnjay/memory"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	distro "github.com/quay/claircore/osrelease"
	log "github.com/sirupsen/logrus"
)

type osMetadata struct {
	OSRelease map[string]string
}

type kernelMetadata struct {
	// VersionInfo struct from moby (docker) provides the following keys
	// for the kernel info:
	// - Kernel int    // Version of the kernel (e.g. 4.1.2-generic -> 4)
	// - Major  int    // Major part of the kernel version (e.g. 4.1.2-generic -> 1)
	// - Minor  int    // Minor part of the kernel version (e.g. 4.1.2-generic -> 2)
	// - Flavor string // Flavor of the kernel version (e.g. 4.1.2-generic -> generic)
	// Recreate them for use with template:
	Kernel, Major, Minor int
	Flavor               string
	Full                 string
}

type cpuMetadata struct {
	NumCPU int // exposes `runtime.NumCPU()` for CPU count in templates
}

type memoryMetadata struct {
	TotalBytes uint64
	FreeBytes  uint64
}

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

func getMemoryMetadata() memoryMetadata {
	return memoryMetadata{
		TotalBytes: memory.TotalMemory(),
		FreeBytes:  memory.FreeMemory(),
	}
}

// storage metadata

const (
	procDir       = "/proc"
	mountInfoFile = procDir + "/self/mountinfo"
	sysDir        = "/sys"
	blockDevDir   = sysDir + "/block"
)

type disk struct {
	Name    string
	Virtual bool
	SSD     bool
}

type storageMetadata struct {
	Mounts []*procfs.MountInfo
	Disks  []disk
}

func getStorageMetadata() storageMetadata {
	storageMD := storageMetadata{}

	fs, err := blockdevice.NewFS(procDir, sysDir)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to create blockdevice FS")
	}

	blockDevs, err := fs.SysBlockDevices()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  blockDevDir,
		}).Error("Failed to list block devices")
	}

	var disks []disk
	for _, blockDev := range blockDevs {
		qStats, err := fs.SysBlockDeviceQueueStats(blockDev)
		log.WithFields(log.Fields{
			"error":  err,
			"device": blockDev,
		}).Error("Failed to get queue stats for block device")

		ssd := false
		if qStats.Rotational == 0 {
			ssd = true
		}

		blockDevPath := filepath.Join(blockDevDir, blockDev)

		blockDevLink, err := os.Readlink(blockDevPath)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"path":  blockDevPath,
			}).Error("Failed to get device link")
		}

		virtual := false
		if strings.Contains(blockDevLink, "virtual") {
			virtual = true
		}

		disks = append(disks, disk{
			Name:    blockDevPath,
			Virtual: virtual,
			SSD:     ssd,
		})
	}

	storageMD.Disks = disks

	mounts, err := procfs.GetMounts()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"path":  mountInfoFile,
		}).Error("Failed to get mounts")
	}
	storageMD.Mounts = mounts

	return storageMD
}
