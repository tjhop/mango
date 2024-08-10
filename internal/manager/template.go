package manager

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"text/template"

	"github.com/go-sprout/sprout"
	socktmpl "github.com/hashicorp/go-sockaddr/template"
	"github.com/oklog/ulid/v2"

	"github.com/tjhop/mango/internal/shell"
)

var (
	once                sync.Once
	sproutFuncMap       template.FuncMap
	sproutDisabledFuncs = []string{"env", "expandenv"}
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

type templateData struct {
	HostVars   VariableMap
	ModuleVars VariableMap
	Vars       VariableMap
	Metadata   metadata
	OS         osMetadata
	Kernel     kernelMetadata
	CPU        cpuMetadata
	Memory     memoryMetadata
	Storage    storageMetadata
}

func init() {
	once.Do(func() {
		sproutFuncMap = sprout.TxtFuncMap()
		for _, f := range sproutDisabledFuncs {
			delete(sproutFuncMap, f)
		}
	})
}

func templateScript(ctx context.Context, path string, view templateView, funcMap template.FuncMap, invDefinedTemplates ...string) (string, error) {
	var (
		buf bytes.Buffer
		err error
	)

	// init template and funcs
	t := template.New(filepath.Base(path)).
		Funcs(funcMap).
		Funcs(socktmpl.SourceFuncs).
		Funcs(socktmpl.SortFuncs).
		Funcs(socktmpl.FilterFuncs).
		Funcs(socktmpl.HelperFuncs).
		Funcs(sproutFuncMap)

	if len(invDefinedTemplates) > 0 {
		if t, err = t.ParseFiles(invDefinedTemplates...); err != nil {
			return "", fmt.Errorf("Failed to parse common templates in %#v: %s", invDefinedTemplates, err)
		}
	}

	t, err = t.ParseFiles(path)
	if err != nil {
		return "", fmt.Errorf("Failed to parse template %s: %s", path, err)
	}

	err = t.Execute(&buf, view)
	if err != nil {
		return "", fmt.Errorf("Failed to execute template for %s: %s", path, err)
	}

	return buf.String(), nil
}

func (mgr *Manager) getTemplateData(ctx context.Context, name string, host, mod, all VariableMap) templateView {
	// runtime metadata for templates
	runtimeData := metadata{
		ModuleName:    name,
		RunID:         ctx.Value(contextKeyRunID).(ulid.ULID).String(),
		Enrolled:      ctx.Value(contextKeyEnrolled).(bool),
		ManagerName:   ctx.Value(contextKeyManagerName).(string),
		InventoryPath: ctx.Value(contextKeyInventoryPath).(string),
		Hostname:      ctx.Value(contextKeyHostname).(string),
	}

	// assemble all template data
	allTemplateData := templateData{
		HostVars:   VariableMap(host),
		ModuleVars: VariableMap(mod),
		Vars:       VariableMap(all),
		Metadata:   runtimeData,
		OS:         mgr.tmplData.OS,
		Kernel:     mgr.tmplData.Kernel,
		CPU:        mgr.tmplData.CPU,
		Memory:     mgr.tmplData.Memory,
		Storage:    mgr.tmplData.Storage,
	}

	return templateView{
		Mango: allTemplateData,
	}
}
