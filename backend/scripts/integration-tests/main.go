// Command integration-tests runs tests introduced by the integration build tag.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type testPackage struct {
	Dir, ImportPath           string
	TestGoFiles, XTestGoFiles []string
}

type testPlan struct {
	Packages []string
	Tests    []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	unit, err := listPackages("unit")
	if err != nil {
		return err
	}
	integration, err := listPackages("integration")
	if err != nil {
		return err
	}
	plan, err := integrationPlan(unit, integration)
	if err != nil {
		return err
	}
	if len(plan.Tests) == 0 {
		return fmt.Errorf("no integration tests selected")
	}
	if len(args) == 1 && args[0] == "--list" {
		return json.NewEncoder(os.Stdout).Encode(plan)
	}
	names := make([]string, len(plan.Tests))
	for i, name := range plan.Tests {
		names[i] = regexp.QuoteMeta(name)
	}
	fmt.Fprintf(os.Stderr, "Integration tests: %d entry points in %d packages\n", len(plan.Tests), len(plan.Packages))
	command := []string{"test", "-tags=integration", "-run=^(" + strings.Join(names, "|") + ")$"}
	command = append(command, args...)
	command = append(command, plan.Packages...)
	cmd := exec.Command("go", command...) //nolint:gosec // G702: trusted local CLI arguments passed to a fixed executable without a shell.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func listPackages(tag string) ([]testPackage, error) {
	cmd := exec.Command("go", "list", "-json=Dir,ImportPath,TestGoFiles,XTestGoFiles", "-tags="+tag, "./...")
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list %s packages: %w", tag, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var packages []testPackage
	for {
		var pkg testPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				return packages, nil
			}
			return nil, err
		}
		packages = append(packages, pkg)
	}
}

func integrationPlan(unit, integration []testPackage) (testPlan, error) {
	shared := make(map[string]bool)
	for _, pkg := range unit {
		for _, files := range [][]string{pkg.TestGoFiles, pkg.XTestGoFiles} {
			for _, file := range files {
				shared[filepath.Join(pkg.Dir, file)] = true
			}
		}
	}
	plan := testPlan{}
	names := make(map[string]bool)
	for _, pkg := range integration {
		selected := false
		for _, files := range [][]string{pkg.TestGoFiles, pkg.XTestGoFiles} {
			for _, file := range files {
				path := filepath.Join(pkg.Dir, file)
				if shared[path] {
					continue
				}
				fileNames, err := testNames(path)
				if err != nil {
					return testPlan{}, err
				}
				for _, name := range fileNames {
					names[name] = true
					selected = true
				}
			}
		}
		if selected {
			plan.Packages = append(plan.Packages, pkg.ImportPath)
		}
	}
	for name := range names {
		plan.Tests = append(plan.Tests, name)
	}
	sort.Strings(plan.Packages)
	sort.Strings(plan.Tests)
	return plan, nil
}

func testNames(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name.Name == "TestMain" {
			continue
		}
		name := function.Name.Name
		for _, prefix := range []string{"Test", "Fuzz"} {
			if suffix, ok := strings.CutPrefix(name, prefix); ok {
				first, _ := utf8.DecodeRuneInString(suffix)
				if suffix == "" || !unicode.IsLower(first) {
					names = append(names, name)
				}
			}
		}
	}
	for _, example := range doc.Examples(file) {
		if example.Output != "" || example.EmptyOutput {
			names = append(names, "Example"+example.Name)
		}
	}
	return names, nil
}
