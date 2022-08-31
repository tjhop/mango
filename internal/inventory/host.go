package inventory

import (
	"path/filepath"
	"time"

	"github.com/tjhop/mango/internal/self"
	"github.com/tjhop/mango/internal/utils"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Host contains fields that represent a given host in the inventory.
// - ID: string idenitfying the host (generally the hostname of the system)
// - Roles: a slice of roles that are applied to this host
// - Modules: a slice of ad-hoc module names applied to this host
// - Variables: variables for the host, in key, value form
type Host struct {
	ID        string
	Modules   []string
	Roles     []string
	Variables VariableMap
}

// GetModules returns a slice of Modules, containing
// all of the modules that are applicable for this host
func (h Host) GetModules() []Module {
	var ml []Module

	for _, moduleID := range h.Modules {
		ml = append(ml, GetModule(moduleID))
	}

	for _, roleID := range h.Roles {
		role := GetRole(roleID)
		ml = append(ml, role.GetModules()...)
	}

	return ml
}

// GetRoles returns a slice of Roles, containing
// all of the roles that are applicable for this host
func (h Host) GetRoles() []Role {
	var roles []Role

	for _, r := range h.Roles {
		roles = append(roles, GetRole(r))
	}

	return roles
}

// GetVariables returns a VariableMap of variables
// assigned to this host
func (h Host) GetVariables() VariableMap {
	return h.Variables
}

// String is a stringer to return the host ID
func (h Host) String() string { return h.ID }

// ParseHosts looks for hosts in the inventory's `hosts/` folder. It looks for
// folders within this directory, and then parses each directory into a Host struct.
// Each host folder is expected to contain files for `apply`, `variables`, and `test`,
// which get set to the corresponding fields in the Host struct for the host.
func (i *Inventory) ParseHosts() error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "hosts",
		"hostname":  self.GetHostname(),
	}

	path := filepath.Join(i.inventoryPath, "hosts")
	hostDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to get files in directory")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadSeconds.With(prometheus.Labels{
			"inventory": commonLabels["inventory"],
			"component": commonLabels["component"],
			"hostname":  commonLabels["hostname"],
			"result":    "failure",
		}).Set(float64(time.Now().Unix()))
		metricInventoryReloadTotal.With(prometheus.Labels{
			"inventory": commonLabels["inventory"],
			"component": commonLabels["component"],
			"hostname":  commonLabels["hostname"],
			"result":    "failure",
		}).Inc()

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
				metricInventoryReloadSeconds.With(prometheus.Labels{
					"inventory": commonLabels["inventory"],
					"component": commonLabels["component"],
					"hostname":  commonLabels["hostname"],
					"result":    "failure",
				}).Set(float64(time.Now().Unix()))
				metricInventoryReloadTotal.With(prometheus.Labels{
					"inventory": commonLabels["inventory"],
					"component": commonLabels["component"],
					"hostname":  commonLabels["hostname"],
					"result":    "failure",
				}).Inc()

				return err
			}

			host := Host{ID: hostPath}

			for _, hostFile := range hostFiles {
				if !hostFile.IsDir() {
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

						host.Roles = roles
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

						host.Modules = mods
					case "variables":
						varsPath := filepath.Join(hostPath, "variables")
						vars, err := ParseVariables(varsPath)
						if err != nil {
							log.WithFields(log.Fields{
								"file":  varsPath,
								"error": err,
							}).Error("Failed to parse variables for module")
						}

						host.Variables = vars
					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}

					hosts = append(hosts, host)
				}
			}
		}
	}

	i.Hosts = hosts
	metricInventory.With(commonLabels).Set(float64(len(i.Hosts)))
	numMyHosts := 0
	if i.IsEnrolled() {
		numMyHosts = 1 // if you're enrolled, you're the host
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(numMyHosts))
	metricInventoryReloadSeconds.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
		"result":    "success",
	}).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
		"result":    "success",
	}).Inc()

	return nil
}
