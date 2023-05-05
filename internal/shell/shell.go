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

// VariableMap is an alias for `map[string]expand.Variable` It's used to store
// variable data in key,value format, using the same format for variables that
// are worked with later with sh lib in the manager
type VariableMap map[string]expand.Variable

// func with const []string return containing environment variables that get
// removed from the environent when sourcing a file
func getEnvVarBlacklist() []string {
	return []string{"PWD", "HOME", "PATH", "IFS", "OPTIND"}
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
func SourceFile(ctx context.Context, path string) (VariableMap, error) {
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

// SourceNode sources a shell program from a node and returns the
// variables declared in it. It accepts the same set of node types that
// interp/Runner.Run does.
//
// This function should be used with caution, as it can interpret arbitrary
// code. Untrusted shell programs shoudn't be sourced outside of a sandbox
// environment.
func SourceNode(ctx context.Context, node syntax.Node) (VariableMap, error) {
	r, _ := interp.New()
	if err := r.Run(ctx, node); err != nil {
		return nil, fmt.Errorf("Failed to run: %v", err)
	}
	// delete the internal shell vars that the user is not
	// interested in
	for _, envVar := range getEnvVarBlacklist() {
		delete(r.Vars, envVar)
	}

	return r.Vars, nil
}

// flattenEnvVarMap is a convenience function that takes a map of raw
// expand.Variables and flattens them all into cmdline compatible `key=value`
// export assignment for ingestion into the interpeter
func flattenEnvVarMap(varMap VariableMap) []string {
	varSlice := []string{}

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

// Run is responsible for assembling an interpreter's execution environment
// (setting environment variables, working directory, IO/output, etc) and
// running the command
func Run(ctx context.Context, runID ulid.ULID, path string, hostVars, modVars VariableMap) error {
	if path == "" {
		return fmt.Errorf("No script path provided")
	}

	// apply variables for the module on top of the host modules to override
	vars := make(VariableMap)
	for k, v := range hostVars {
		vars[k] = v
	}
	for k, v := range modVars {
		vars[k] = v
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
		interp.Env(expand.ListEnviron(append(os.Environ(), flattenEnvVarMap(vars)...)...)),
		interp.StdIO(nil, stdoutLog, stderrLog),
		interp.Dir(workDir),
	)
	if err != nil {
		return fmt.Errorf("Failed to create shell interpreter: %s", err)
	}

	// run script through template
	renderedScript, err := templateScript(ctx, path, templateData{
		HostVars:   hostVars,
		ModuleVars: modVars,
		Vars:       vars,
	})
	if err != nil {
		return fmt.Errorf("Failed to template script: %s", err)
	}

	// create shell parser based on rendered template script
	file, err := syntax.NewParser().Parse(strings.NewReader(renderedScript), path)
	if err != nil {
		return fmt.Errorf("Failed to parse: %v", err)
	}

	// run it!
	if err = runner.Run(ctx, file); err != nil {
		return fmt.Errorf("Failed to run script %s: %v", path, err)
	}

	return nil
}
