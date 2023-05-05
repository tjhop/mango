package shell

import (
	"bytes"
	"context"
	"path/filepath"
	"text/template"
)

type templateData struct {
	HostVars   VariableMap
	ModuleVars VariableMap
	Vars       VariableMap
}

func templateScript(ctx context.Context, path string, data templateData) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New(filepath.Base(path)).ParseFiles(path))
	err := t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
