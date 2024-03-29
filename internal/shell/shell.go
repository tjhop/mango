package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/viper"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// VariableSlice is an alias for `[]string`, where each item is an environment
// variable in `key=value` form, the same as returned by `os.Environ()`
type VariableSlice []string

// VariableMap is an alias for `map[string]string`, where the key is the
// variable name and the string is the variable value
type VariableMap map[string]string

// func with const []string return containing environment variables that get
// removed from the environent when sourcing a file
func getEnvVarBlacklist() []string {
	return []string{"PWD", "HOME", "PATH", "IFS", "OPTIND", "GID", "UID"}
}

// SourceFile()/SourceNode() functions inspired heavily by old convenience
// functions in mvdan/sh source, and updated to work with v3/this project as
// needed:
// https://raw.githubusercontent.com/mvdan/sh/v2.6.4/shell/source.go

// --------------------

// SourceFile sources a shell file from disk and returns the variables
// declared in it. It is a convenience function that uses a default shell
// parser, parses a file from disk, and calls SourceNode.
//
// This function should be used with caution, as it can interpret arbitrary
// code. Untrusted shell programs shoudn't be sourced outside of a sandbox
// environment.
func SourceFile(ctx context.Context, path string) (VariableSlice, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open: %v", err)
	}
	defer f.Close()
	file, err := syntax.NewParser().Parse(f, path)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse: %v", err)
	}
	return SourceNode(ctx, file)
}

// SourceNode sources a shell program from a node and returns the variables
// declared in it. Variables are returned in the same format as `os.Environ()`
// -- ie, `[]string{"key=value",...}`. It accepts the same set of node types
// that interp/Runner.Run does.
//
// This function should be used with caution, as it can interpret arbitrary
// code. Untrusted shell programs shoudn't be sourced outside of a sandbox
// environment.
func SourceNode(ctx context.Context, node syntax.Node) (VariableSlice, error) {
	r, _ := interp.New()

	// take initial copy of environment variables
	oldVars := os.Environ()

	if err := r.Run(ctx, node); err != nil {
		return nil, fmt.Errorf("Failed to run: %v", err)
	}

	newVars := getUpdatedVars(oldVars, flattenEnvVarMap(r.Vars))
	var filteredVars VariableSlice
	for _, v := range newVars {
		found := false
		for _, x := range getEnvVarBlacklist() {
			if strings.HasPrefix(v, x) {
				found = true
				break
			}
		}

		if !found {
			// only return the variable if it isn't found on the blacklist
			filteredVars = append(filteredVars, v)
		}
	}

	return filteredVars, nil
}

func getUpdatedVars(oldVars, newVars VariableSlice) VariableSlice {
	var keep VariableSlice

	// compare envs before and after and only return the new/updated variables
	for _, newKV := range newVars {
		newTokens := strings.Split(newKV, "=")
		if len(newTokens) != 2 {
			continue
		}
		nKey := newTokens[0]
		nValue := newTokens[1]
		found := false

		for _, oldKV := range oldVars {
			oldTokens := strings.Split(oldKV, "=")
			if len(oldTokens) != 2 {
				continue
			}
			oKey := oldTokens[0]
			oValue := oldTokens[1]

			if nKey == oKey {
				found = true
				if nValue != oValue {
					// var was updated new running, it has a new value -- keep it
					keep = append(keep, newKV)
				}
				break
			}
		}

		if !found {
			// if a var name found new running doesn't have a
			// corresponding var name found in the vars before
			// running, it's a new variable -- keep it
			keep = append(keep, newKV)
		}
	}

	return keep
}

// flattenEnvVarMap is a convenience function that takes a map of raw
// expand.Variables and flattens them all into cmdline compatible `key=value`
// export assignment for ingestion into the interpeter.
// This function is also used as a conversion step to resolve and transform the
// variables into simpler data types with their end-state values.
func flattenEnvVarMap(varMap map[string]expand.Variable) VariableSlice {
	varSlice := VariableSlice{}

	for name, data := range varMap {
		switch data.Kind {
		case expand.String, expand.NameRef:
			varSlice = append(varSlice, fmt.Sprintf("%s=%s", name, data.Str))
			// also flatten indexed + associative arrays (ie, arrays and maps)
		case expand.Indexed:
			varSlice = append(varSlice, fmt.Sprintf("%s=( %s )", name, strings.Join(data.List, " ")))
		case expand.Associative:
			for k, v := range data.Map {
				varSlice = append(varSlice, fmt.Sprintf("%s[%s]=%s", name, k, v))
			}
		}
	}

	return varSlice
}

// MakeVariableMap is a convenience function to conert a `VariableSlice` to a
// `VariableMap`
func MakeVariableMap(varSlice VariableSlice) VariableMap {
	varMap := make(VariableMap)
	for _, v := range varSlice {
		tokens := strings.Split(v, "=")
		if len(tokens) != 2 {
			continue
		}
		varMap[tokens[0]] = tokens[1]
	}
	return varMap
}

func MergeVariables(maps ...VariableMap) VariableSlice {
	vars := make(VariableMap)

	for _, varMap := range maps {
		for k, v := range varMap {
			vars[k] = v
		}
	}

	varSlice := make(VariableSlice, len(vars))
	for k, v := range vars {
		varSlice = append(varSlice, fmt.Sprintf("%s=%s", k, v))
	}

	return varSlice
}

// Run is responsible for assembling an interpreter's execution environment
// (setting environment variables, working directory, IO/output, etc) and
// running the command
// Accepts:
//   - context
//   - ULID specific to this run
//   - path to the script
//   - string containing the contents of the templated script
//   - a slice of strings in `key=value` pair containing the merged variables to
//     be provided to the script as environment variables
func Run(ctx context.Context, runID ulid.ULID, path, content string, allVars []string) error {
	if content == "" {
		return fmt.Errorf("No script data provided")
	}

	// setup log files for script output
	// format of paths:
	//	/var/log/mango/manager/run/$runID/$module/$file
	// example path (started with `inventory.path`: './test/mockup/inventory'):
	//	/var/log/mango/manager/run/01GZF2QSPGTCKHFSECPBQ6H8FQ/test/mockup/inventory/modules/test-env-vars/apply/stdout
	logDir := filepath.Join(viper.GetString("mango.log-dir"), "manager/run", runID.String(), path)
	if err := os.MkdirAll(logDir, 0750); err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create directory for script logs: %v", err)
	}
	stdoutLog, err := os.OpenFile(filepath.Join(logDir, "stdout"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open script log for stdout: %v", err)
	}
	defer stdoutLog.Close()

	// log stderr from script
	// `mango_$scriptID_timestamp_stderr.log
	stderrLog, err := os.OpenFile(filepath.Join(logDir, "stderr"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open script log for stderr: %v", err)
	}
	defer stderrLog.Close()

	// runtime dir prep
	workDir := filepath.Join(viper.GetString("mango.temp-dir"), runID.String())
	if err := os.MkdirAll(workDir, 0750); err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create working directory for script: %v", err)
	}

	// create shell interpreter
	runner, err := interp.New(
		interp.Env(expand.ListEnviron(append(os.Environ(), allVars...)...)),
		interp.StdIO(nil, stdoutLog, stderrLog),
		interp.Dir(workDir),
	)
	if err != nil {
		return fmt.Errorf("Failed to create shell interpreter: %s", err)
	}

	// create shell parser based on rendered template script
	file, err := syntax.NewParser().Parse(strings.NewReader(content), path)
	if err != nil {
		return fmt.Errorf("Failed to parse: %v", err)
	}

	// run it!
	if err = runner.Run(ctx, file); err != nil {
		return fmt.Errorf("Failed to run script %s: %v", path, err)
	}

	return nil
}
