package manager

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	socktmpl "github.com/hashicorp/go-sockaddr/template"
	log "github.com/sirupsen/logrus"
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
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"panic": r,
			}).Error("Failed to template script")
		}
	}()

	var buf bytes.Buffer
	t := template.Must(template.New(filepath.Base(path)).Funcs(funcMap).ParseFiles(path))
	err := t.Execute(&buf, view)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// custom template functions

// sockaddrTemplate is a wrapper around Hashicorp's sockaddr library's
// template.Parse() function. all this wrapper does is take the raw text of the
// template and wrap it in `{{ }}` to feed into the sockaddr template Parse()
// funtion.
func sockaddrTemplate(tmpl ...string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"panic": r,
			}).Error("Failed to parse sockaddr template")
		}
	}()

	return socktmpl.Parse(fmt.Sprintf("{{ %s }}", strings.Join(tmpl, " ")))
}
