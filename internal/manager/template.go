package manager

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"text/template"

	socktmpl "github.com/hashicorp/go-sockaddr/template"
	"github.com/tjhop/mango/internal/shell"
)

type VariableSlice = shell.VariableSlice
type VariableMap = shell.VariableMap

type templateView struct {
	Mango templateData
}

type metadata struct {
	ModuleName    string // name of the module/directive executing the template
	Enrolled      bool
	RunID         string
	ManagerName   string
	InventoryPath string
	Hostname      string
}

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
}

type templateData struct {
	HostVars   VariableMap
	ModuleVars VariableMap
	Vars       VariableMap
	Metadata   metadata
	OS         osMetadata
	Kernel     kernelMetadata
}

func templateScript(ctx context.Context, path string, view templateView, funcMap template.FuncMap) (string, error) {
	var buf bytes.Buffer
	t, err := template.New(filepath.Base(path)).
		Funcs(funcMap).
		Funcs(socktmpl.SourceFuncs).
		Funcs(socktmpl.SortFuncs).
		Funcs(socktmpl.FilterFuncs).
		Funcs(socktmpl.HelperFuncs).
		ParseFiles(path)
	if err != nil {
		return "", fmt.Errorf("Failed to parse template %s: %s", path, err)
	}

	err = t.Execute(&buf, view)
	if err != nil {
		return "", fmt.Errorf("Failed to execute template for %s: %s", path, err)
	}

	return buf.String(), nil
}

// custom template functions
