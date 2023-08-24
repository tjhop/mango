package inventory

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjhop/mango/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Host contains fields that represent a given host in the inventory.
// - id: string idenitfying the host (generally the hostname of the system)
// - roles: a slice of roles that are applied to this host
// - modules: a slice of ad-hoc module names applied to this host
// - variables: path to the variables file for this host, if present
type Host struct {
	id        string
	modules   []string
	roles     []string
	variables string
}

// String is a stringer to return the host ID
func (h Host) String() string { return h.id }

// ParseHosts looks for hosts in the inventory's `hosts/` folder. It looks for
// folders within this directory, and then parses each directory into a Host struct.
// Each host folder is expected to contain files for `apply`, `variables`, and `test`,
// which get set to the corresponding fields in the Host struct for the host.
func (i *Inventory) ParseHosts(ctx context.Context) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "hosts",
	}

	path := filepath.Join(i.inventoryPath, "hosts")
	hostDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to get files in directory")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var hosts []Host

	for _, hostDir := range hostDirs {
		if hostDir.IsDir() {
			hostPath := filepath.Join(path, hostDir.Name())
			hostFiles, err := utils.GetFilesInDirectory(hostPath)
			if err != nil {
				log.WithFields(log.Fields{
					"path":  hostPath,
					"error": err,
				}).Error("Failed to parse host files")

				// inventory counts haven't been altered, no need to update here
				metricInventoryReloadFailedTotal.With(commonLabels).Inc()

				return err
			}

			host := Host{id: hostDir.Name()}

			for _, hostFile := range hostFiles {
				if !hostFile.IsDir() && !strings.HasPrefix(hostFile.Name(), ".") {
					fileName := hostFile.Name()
					switch fileName {
					case "roles":
						var roles []string
						rolePath := filepath.Join(hostPath, "roles")
						lines := utils.ReadFileLines(rolePath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  rolePath,
									"error": line.Err,
								}).Error("Failed to read roles for host")
							} else {
								roles = append(roles, line.Text)
							}
						}

						host.roles = roles
					case "modules":
						var mods []string
						modPath := filepath.Join(hostPath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  modPath,
									"error": line.Err,
								}).Error("Failed to read modules for host")
							} else {
								mods = append(mods, line.Text)
							}
						}

						host.modules = mods
					case "variables":
						host.variables = filepath.Join(hostPath, "variables")
					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}
				}
			}

			hosts = append(hosts, host)
		}
	}

	i.hosts = hosts
	metricInventory.With(commonLabels).Set(float64(len(i.hosts)))
	numMyHosts := 0
	if i.IsEnrolled() {
		numMyHosts = 1 // if you're enrolled, you're the host
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(numMyHosts))
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}
