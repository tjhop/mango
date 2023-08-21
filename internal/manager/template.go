package manager

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"text/template"

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
	Full                 string
}

type templateData struct {
	HostVars   VariableMap
	ModuleVars VariableMap
	Vars       VariableMap
	Metadata   metadata
	OS         osMetadata
	Kernel     kernelMetadata
}

// namespaceTemplateFuncs prefixes all function.
// ie, `GetPublicIP` from the sockaddr template custom functions is made
// available as `sockaddr_GetPublicIP`, etc
func namespaceTemplateFuncs(namespace string, in template.FuncMap) template.FuncMap {
	out := make(template.FuncMap)
	for k, v := range in {
		out[fmt.Sprintf("%s_%s", namespace, k)] = v
	}

	return out
}

func templateScript(ctx context.Context, path string, view templateView, funcMap template.FuncMap) (string, error) {
	var buf bytes.Buffer
	t, err := template.New(filepath.Base(path)).
		Funcs(funcMap).
		Funcs(namespaceTemplateFuncs("sockaddr", socktmpl.SourceFuncs)).
		Funcs(namespaceTemplateFuncs("sockaddr", socktmpl.SortFuncs)).
		Funcs(namespaceTemplateFuncs("sockaddr", socktmpl.FilterFuncs)).
		Funcs(namespaceTemplateFuncs("sockaddr", socktmpl.HelperFuncs)).
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
	}

	return templateView{
		Mango: allTemplateData,
	}
}

// custom template functions

// isIPv4 returns true if the given string is an IPv4 address and false otherwise
func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}

	ip4 := ip.To4()
	return ip4 != nil
}

// isIPv6 returns true if the given string is an IPv6 address and false otherwise
func isIPv6(s string) bool {
	// short circuit if it's an IPv4
	if isIPv4(s) {
		return false
	}

	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}

	ip6 := ip.To16()
	return ip6 != nil
}
