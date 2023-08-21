package inventory

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tjhop/mango/internal/utils"

	glob_util "github.com/gobwas/glob"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Group contains fields that represent a given group of groups in the inventory.
// - id: string idenitfying the group
// - globs: a slice of glob patterns to match against the instance's hostname
// - patterns: a slice of regex patterns to match against the instance's hostname
// - roles: a slice of roles that are applied to this host
// - modules: a slice of ad-hoc module names applied to this host
// - variables: path to the variables file for this group, if present
type Group struct {
	id        string
	globs     []string
	patterns  []string
	modules   []string
	roles     []string
	variables string
}

// String is a stringer to return the group ID
func (g Group) String() string { return g.id }

// ParseGroups looks for groups in the inventory's `groups/` folder. It looks for
// folders within this directory, and then parses each directory into a Group struct.
// Each Group folder may contain a file `glob` containing a newline separated
// list of glob matches, and a `regex` file containing regular expression
// patterns for comparing groupnames.
func (i *Inventory) ParseGroups(ctx context.Context) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "groups",
	}

	path := filepath.Join(i.inventoryPath, "groups")
	groupDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to get files in directory")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var groups []Group

	for _, groupDir := range groupDirs {
		if groupDir.IsDir() {
			groupPath := filepath.Join(path, groupDir.Name())
			groupFiles, err := utils.GetFilesInDirectory(groupPath)
			if err != nil {
				log.WithFields(log.Fields{
					"path":  groupPath,
					"error": err,
				}).Error("Failed to parse group files")

				// inventory counts haven't been altered, no need to update here
				metricInventoryReloadFailedTotal.With(commonLabels).Inc()

				return err
			}

			group := Group{id: groupDir.Name()}

			for _, groupFile := range groupFiles {
				if !groupFile.IsDir() && !strings.HasPrefix(groupFile.Name(), ".") {
					fileName := groupFile.Name()
					switch fileName {
					case "glob":
						var globs []string
						globPath := filepath.Join(groupPath, "glob")
						lines := utils.ReadFileLines(globPath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  globPath,
									"error": line.Err,
								}).Error("Failed to read globs for group")
							} else {
								globs = append(globs, line.Text)
							}
						}

						group.globs = globs
					case "regex":
						var patterns []string
						patternPath := filepath.Join(groupPath, "regex")
						lines := utils.ReadFileLines(patternPath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  patternPath,
									"error": line.Err,
								}).Error("Failed to read regexs for group")
							} else {
								patterns = append(patterns, line.Text)
							}
						}

						group.patterns = patterns
					case "roles":
						var roles []string
						rolePath := filepath.Join(groupPath, "roles")
						lines := utils.ReadFileLines(rolePath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  rolePath,
									"error": line.Err,
								}).Error("Failed to read roles for group")
							} else {
								roles = append(roles, line.Text)
							}
						}

						group.roles = roles
					case "modules":
						var mods []string
						modPath := filepath.Join(groupPath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  modPath,
									"error": line.Err,
								}).Error("Failed to read modules for group")
							} else {
								mods = append(mods, line.Text)
							}
						}

						group.modules = mods
					case "variables":
						group.variables = filepath.Join(groupPath, "variables")
					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}
				}
			}

			groups = append(groups, group)
		}
	}

	i.groups = groups
	metricInventory.With(commonLabels).Set(float64(len(i.groups)))
	groupMatches := 0
	for _, group := range i.groups {
		if group.IsHostEnrolled(i.hostname) {
			groupMatches++
		}
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(groupMatches))
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}

func (g Group) MatchGlobs(hostname string) int {
	matched := 0

	for _, globPattern := range g.globs {
		glob := glob_util.MustCompile(globPattern)
		if glob.Match(hostname) {
			matched++
			continue
		}
	}

	return matched
}

func (g Group) MatchPatterns(hostname string) int {
	matched := 0

	for _, pattern := range g.patterns {
		validPattern := regexp.MustCompile(pattern)
		if validPattern.MatchString(hostname) {
			matched++
			continue
		}
	}

	return matched
}

func (g Group) MatchAll(hostname string) int {
	return g.MatchGlobs(hostname) + g.MatchPatterns(hostname)
}

func (g Group) IsHostEnrolled(hostname string) bool {
	return g.MatchAll(hostname) > 0
}