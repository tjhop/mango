package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	addCmdAliases  = []string{"create", "init", "new"}
	delCmdAliases  = []string{"remove", "rm", "del"}
	listCmdAliases = []string{"show", "print", "ls"}
)

func inventoryAddFile(name string) error {
	file, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("Error opening file <%s>: %w", name, err)
	}
	return file.Close()
}

func inventoryAddDir(name string) error {
	if err := os.MkdirAll(name, 0o755); err != nil {
		return fmt.Errorf("Error making directory <%s>: %w", name, err)
	}

	if err := inventoryAddFile(filepath.Join(name, ".gitkeep")); err != nil {
		return fmt.Errorf("Error adding directory file <%s>: %w", name, err)
	}

	return nil
}

func inventoryRemoveAll(name string) error {
	err := os.RemoveAll(name)
	if err != nil {
		return fmt.Errorf("Error recursively removing <%s>: %w", name, err)
	}

	return nil
}

type urlParam struct {
	key   string
	value string
}

func httpGetBody(addr string, path string, urlParams []urlParam) (string, error) {
	if !strings.HasPrefix(addr, "http") {
		addr = "http://" + addr
	}

	pprofUrl, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("Error parsing url <%s>: %w", addr, err)
	}

	if pprofUrl.Scheme == "" {
		pprofUrl.Scheme = "http"
	}

	pprofUrl.Path += path

	params := url.Values{}
	for _, p := range urlParams {
		params.Add(p.key, p.value)
	}
	pprofUrl.RawQuery = params.Encode()

	// fmt.Printf("TJ DEBUG | assembled url is: %s\n", pprofUrl.String())
	res, err := http.Get(pprofUrl.String())
	if err != nil {
		return "", fmt.Errorf("Error making HTTP Get request to <%s>: %w", addr, err)
	}

	body, err := io.ReadAll(res.Body)
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return string(body), fmt.Errorf("HTTP response failed (status code: %d)", res.StatusCode)
	}
	if err != nil {
		return "", fmt.Errorf("Error reading HTTP response body: %w", err)
	}

	return string(body), nil
}
