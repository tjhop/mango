package inventory

import (
	"strings"

	"github.com/tjhop/mango/internal/utils"

	log "github.com/sirupsen/logrus"
)

// VariableMap is an alias for `map[string]string`
// It's used to store variable data in key,value format.
type VariableMap map[string]string

// ParseVariables reads the file at the given `path` and parses each line into
// a map of variables. The key of the map is the variable name, and the value is
// the value of the variable. If values are quoted, the quotes are removed.
func ParseVariables(path string) (VariableMap, error) {
	vars := make(VariableMap)
	lines := utils.ReadFileLines(path)

	for line := range lines {
		if line.Err != nil {
			log.WithFields(log.Fields{
				"path":  path,
				"error": line.Err,
			}).Error("Failed to read variables file")
		} else {
			tokens := strings.Split(line.Text, "=")
			if len(tokens) != 2 {
				// this is a variables file, which might contain sensitive data.
				// do not log the contents of the scanned lines.
				log.WithFields(log.Fields{
					"path": path,
				}).Warning("Potentially malformed line in variables file in inventory")
			} else {
				varName := strings.TrimSpace(tokens[0])
				varValue := strings.TrimSpace(strings.Trim(tokens[1], "\"'"))
				vars[varName] = varValue
			}
		}
	}

	return vars, nil
}
