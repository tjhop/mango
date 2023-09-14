package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	kernelParser "github.com/moby/moby/pkg/parsers/kernel"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	distro "github.com/quay/claircore/osrelease"
	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/pkg/utils"
)

// general

const (
	procDir = "/proc"
	sysDir  = "/sys"
)

// OS metadata

type osMetadata struct {
	OSRelease map[string]string
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

// kernel metadata

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

// CPU metadata
type cpuMetadata struct {
	Cores []procfs.CPUInfo
}

func getCPUMetadata() cpuMetadata {
	fs, err := procfs.NewFS(procDir)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to create procfs for cpu metadata")
	}

	cpuInfo, err := fs.CPUInfo()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to read cpu info")
	}

	return cpuMetadata{Cores: cpuInfo}
}

// memory metadata
var (
	meminfoFile = procDir + "/meminfo"
	reParens    = regexp.MustCompile(`\((.*)\)`)
)

// meminfo collection heavily inspired by meminfo collector in prometheus node exporter
// https://github.com/prometheus/node_exporter/blob/f34aaa61092fe7e3c6618fdb0b0d16a68a291ff7/collector/meminfo_linux.go
//
// was going to use prometheus/procfs's `Meminfo` struct and the `fs.Meminfo()`
// method to get meminfo, but it explicitly calls out in the code that it
// desregards the unit field, which could lead to inaccurate results
//
// https://pkg.go.dev/github.com/prometheus/procfs#Meminfo
// https://pkg.go.dev/github.com/prometheus/procfs#FS.Meminfo
// https://github.com/prometheus/procfs/blob/113c5013dda3c600bda241d86c64258ec7117c7b/meminfo.go#L165
// https://github.com/prometheus/procfs/issues/565

type memoryMetadata map[string]uint64

func getMemoryMetadata() memoryMetadata {
	var memoryMD = make(memoryMetadata)
	lines := utils.ReadFileLines(meminfoFile)
	for line := range lines {
		fields := strings.Fields(line.Text)

		// ignore empty lines
		if len(fields) == 0 {
			continue
		}

		// normalize field names, they need to be easily usable in a template
		// Active(anon) -> Active_anon, etc
		key := strings.TrimSuffix(fields[0], ":")
		key = reParens.ReplaceAllString(key, "_${1}")
		var val uint64

		switch len(fields) {
		case 2:
			// no unit provided, parse directly as bytes
			fv, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
					"entry": line.Text,
				}).Error("Failed to create meminfo, invalid value")
			}
			val = fv
		case 3:
			// unit provided, parse into bytes via humanize
			parsedBytes, err := humanize.ParseBytes(fmt.Sprintf("%s %s", fields[1], fields[2]))
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
					"entry": line.Text,
				}).Error("Failed to parse unit in meminfo")

				continue
			}

			val = parsedBytes
		default:
			// malformed line, wrong number of fields (ie, a single field returned or a 4+ returned)
			// log and continue
			log.WithFields(log.Fields{
				"entry": line.Text,
			}).Warn("Failed to parse meminfo entry, possibly malformed")

			continue
		}

		memoryMD[key] = val
	}

	return memoryMD
}

// storage metadata

const (
	mountInfoFile = procDir + "/self/mountinfo"
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
		blockDevPath := filepath.Join(blockDevDir, blockDev)

		qStats, err := fs.SysBlockDeviceQueueStats(blockDevPath)
		log.WithFields(log.Fields{
			"error":  err,
			"device": blockDev,
		}).Error("Failed to get queue stats for block device")

		ssd := false
		if qStats.Rotational == 0 {
			ssd = true
		}

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
