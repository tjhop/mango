package manager

import (
	"bytes"
	"context"
	"path/filepath"
	"text/template"

	"github.com/tjhop/mango/internal/shell"
)

type VariableSlice = shell.VariableSlice
type VariableMap = shell.VariableMap

type templateView struct {
	Mango templateData
}

type metadata struct {
	Enrolled      bool
	RunID         string
	ManagerName   string
	InventoryPath string
	Hostname      string
}

type templateData struct {
	HostVars   VariableMap
	ModuleVars VariableMap
	Vars       VariableMap
	Metadata   metadata
}

func templateScript(ctx context.Context, path string, view templateView) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New(filepath.Base(path)).ParseFiles(path))
	err := t.Execute(&buf, view)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
