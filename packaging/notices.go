// Collect license notices for the Go modules linked into the two helpers.
// This authoring tool uses only the standard library and ships no runtime code.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type module struct {
	Path, Version, Dir string
	Main               bool
	Replace            *module
}

func main() {
	if err := collect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func collect() error {
	modules := map[string]module{}
	for _, directory := range os.Args[1:] {
		command := exec.Command("go", "list", "-deps", "-json", "-tags", "sqlite_fts5", "./cmd/...")
		command.Dir = directory
		command.Stderr = os.Stderr
		output, err := command.Output()
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		for {
			var pkg struct{ Module *module }
			if err := decoder.Decode(&pkg); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if pkg.Module == nil || pkg.Module.Main {
				continue
			}
			m := *pkg.Module
			if m.Replace != nil {
				m = *m.Replace
			}
			modules[m.Path+"@"+m.Version] = m
		}
	}
	keys := make([]string, 0, len(modules))
	for key := range modules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Println("LocalWhatsAppMCP — third-party Go module notices")
	fmt.Println("The separately bundled wacli MIT notice is in wacli-LICENSE.txt.")
	for _, key := range keys {
		m := modules[key]
		fmt.Printf("\n===== %s =====\n", key)
		entries, err := os.ReadDir(m.Dir)
		if err != nil {
			return fmt.Errorf("read dependency license directory for %s: %w", key, err)
		}
		for _, entry := range entries {
			name := strings.ToUpper(entry.Name())
			if entry.IsDir() || !(strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "LICENCE") || strings.HasPrefix(name, "NOTICE") || strings.HasPrefix(name, "COPYING")) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(m.Dir, entry.Name()))
			if err != nil {
				return err
			}
			fmt.Printf("\n--- %s ---\n%s\n", entry.Name(), data)
		}
	}
	command := exec.Command("go", "env", "GOROOT")
	output, err := command.Output()
	if err != nil {
		return err
	}
	goRoot := strings.TrimSpace(string(output))
	license, err := os.ReadFile(filepath.Join(goRoot, "LICENSE"))
	if os.IsNotExist(err) {
		// Homebrew places the distribution notice alongside libexec (GOROOT).
		license, err = os.ReadFile(filepath.Join(filepath.Dir(goRoot), "LICENSE"))
	}
	if err != nil {
		return err
	}
	fmt.Printf("\n===== Go runtime =====\n%s\n", license)
	return nil
}
