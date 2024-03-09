package manager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	kernelParser "github.com/moby/moby/pkg/parsers/kernel"
	"github.com/prometheus/procfs"
	"github.com/prometheus/procfs/blockdevice"
	distro "github.com/quay/claircore/osrelease"
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

func getOSMetadata(ctx context.Context, logger *slog.Logger) osMetadata {
	// os metadata for templates
	logger = logger.With(
		slog.String("metadata_collector", "os"),
	)

	osReleaseFile, err := os.Open(distro.Path)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to open os-release file",
			slog.String("err", err.Error()),
			slog.String("path", distro.Path),
		)
	}
	osRelease, err := distro.Parse(ctx, osReleaseFile)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to parse os-release file",
			slog.String("err", err.Error()),
			slog.String("path", distro.Path),
		)
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

func getKernelMetadata(ctx context.Context, logger *slog.Logger) kernelMetadata {
	logger = logger.With(
		slog.String("metadata_collector", "kernel"),
	)

	// kernel metadata for templates
	kernelInfo, err := kernelParser.GetKernelVersion()
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to parse kernel info",
			slog.String("err", err.Error()),
		)
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

func getCPUMetadata(ctx context.Context, logger *slog.Logger) cpuMetadata {
	logger = logger.With(
		slog.String("metadata_collector", "cpu"),
	)

	fs, err := procfs.NewFS(procDir)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to create procfs for cpu metadata",
			slog.String("err", err.Error()),
			slog.String("path", procDir),
		)
	}

	cpuInfo, err := fs.CPUInfo()
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to read cpu info",
			slog.String("err", err.Error()),
		)
	}

	return cpuMetadata{Cores: cpuInfo}
}

type memoryMetadata struct {
	procfs.Meminfo
}

func getMemoryMetadata(ctx context.Context, logger *slog.Logger) memoryMetadata {
	logger = logger.With(
		slog.String("metadata_collector", "memory"),
	)

	fs, err := procfs.NewFS(procDir)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to create procfs for memory metadata",
			slog.String("err", err.Error()),
			slog.String("path", procDir),
		)
	}

	memInfo, err := fs.Meminfo()
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to read memory info",
			slog.String("err", err.Error()),
		)
	}

	return memoryMetadata{memInfo}
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

func getStorageMetadata(ctx context.Context, logger *slog.Logger) storageMetadata {
	logger = logger.With(
		slog.String("metadata_collector", "storage"),
	)

	storageMD := storageMetadata{}

	fs, err := blockdevice.NewFS(procDir, sysDir)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to create blockdevice FS",
			slog.String("err", err.Error()),
		)
	}

	blockDevs, err := fs.SysBlockDevices()
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to list block devices",
			slog.String("err", err.Error()),
			slog.String("path", blockDevDir),
		)
	}

	var disks []disk
	for _, blockDev := range blockDevs {
		blockDevPath := filepath.Join(blockDevDir, blockDev)

		qStats, err := fs.SysBlockDeviceQueueStats(blockDevPath)
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to get queue stats for block device",
			slog.String("err", err.Error()),
			slog.String("device", blockDev),
		)

		ssd := false
		if qStats.Rotational == 0 {
			ssd = true
		}

		blockDevLink, err := os.Readlink(blockDevPath)
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to get device link",
				slog.String("err", err.Error()),
				slog.String("path", blockDevPath),
			)
		}

		virtual := strings.Contains(blockDevLink, "virtual")

		disks = append(disks, disk{
			Name:    blockDevPath,
			Virtual: virtual,
			SSD:     ssd,
		})
	}

	storageMD.Disks = disks

	mounts, err := procfs.GetMounts()
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to get mounts",
			slog.String("err", err.Error()),
			slog.String("path", mountInfoFile),
		)
	}
	storageMD.Mounts = mounts

	return storageMD
}
