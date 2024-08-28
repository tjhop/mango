package manager

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/registry/checksum"
	"github.com/go-sprout/sprout/registry/conversion"
	"github.com/go-sprout/sprout/registry/crypto"
	"github.com/go-sprout/sprout/registry/encoding"
	"github.com/go-sprout/sprout/registry/filesystem"
	"github.com/go-sprout/sprout/registry/maps"
	"github.com/go-sprout/sprout/registry/numeric"
	"github.com/go-sprout/sprout/registry/random"
	"github.com/go-sprout/sprout/registry/reflect"
	"github.com/go-sprout/sprout/registry/regexp"
	"github.com/go-sprout/sprout/registry/semver"
	"github.com/go-sprout/sprout/registry/slices"
	"github.com/go-sprout/sprout/registry/std"
	"github.com/go-sprout/sprout/registry/strings"
	"github.com/go-sprout/sprout/registry/time"
	"github.com/go-sprout/sprout/registry/uniqueid"
	socktmpl "github.com/hashicorp/go-sockaddr/template"
	"github.com/oklog/ulid/v2"

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

func templateScript(ctx context.Context, path string, view templateView, funcMap template.FuncMap, invDefinedTemplates ...string) (string, error) {
	var (
		buf bytes.Buffer
		err error
	)

	handler := sprout.New()
	if err := handler.AddRegistries(
		checksum.NewRegistry(),
		conversion.NewRegistry(),
		crypto.NewRegistry(),
		encoding.NewRegistry(),
		filesystem.NewRegistry(),
		maps.NewRegistry(),
		numeric.NewRegistry(),
		random.NewRegistry(),
		reflect.NewRegistry(),
		regexp.NewRegistry(),
		semver.NewRegistry(),
		slices.NewRegistry(),
		std.NewRegistry(),
		strings.NewRegistry(),
		time.NewRegistry(),
		uniqueid.NewRegistry(),
	); err != nil {
		return "", fmt.Errorf("Failed to add sprout registries to handler: %s\n", err.Error())
	}

	// init template and funcs
	t := template.New(filepath.Base(path)).
		Funcs(funcMap).
		Funcs(socktmpl.SourceFuncs).
		Funcs(socktmpl.SortFuncs).
		Funcs(socktmpl.FilterFuncs).
		Funcs(socktmpl.HelperFuncs).
		Funcs(handler.Build())

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
