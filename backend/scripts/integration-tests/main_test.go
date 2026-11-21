//go:build unit

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIntegrationPlanSelectsFilesRatherThanNames(t *testing.T) {
	dir := t.TempDir()
	for file, source := range map[string]string{
		"shared_test.go":      "package fixture; func TestShared(t *testing.T) {}",
		"unit_test.go":        "package fixture; func TestSameName(t *testing.T) {}",
		"integration_test.go": "package fixture; func TestSameName(t *testing.T) {}; func TestDatabase(t *testing.T) {}",
		"external_test.go":    "package fixture_test; func TestExternal(t *testing.T) {}",
		"harness_test.go":     "package fixture; func TestMain(m *testing.M) {}; func helper() {}",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	unit := []testPackage{{Dir: dir, ImportPath: "fixture", TestGoFiles: []string{"shared_test.go", "unit_test.go"}}}
	integration := []testPackage{{Dir: dir, ImportPath: "fixture", TestGoFiles: []string{"shared_test.go", "integration_test.go", "harness_test.go"}, XTestGoFiles: []string{"external_test.go"}}}
	plan, err := integrationPlan(unit, integration)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Packages, []string{"fixture"}) || !reflect.DeepEqual(plan.Tests, []string{"TestDatabase", "TestExternal", "TestSameName"}) {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	plan, err = integrationPlan(unit, unit)
	if err != nil || len(plan.Tests) != 0 || len(plan.Packages) != 0 {
		t.Fatalf("shared files must not select any tests: %+v, %v", plan, err)
	}
}

func TestTestNamesIncludesFuzzAndRunnableExamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entry_test.go")
	source := `package fixture
func Test(t *testing.T) {}
func Test_边界(t *testing.T) {}
func Testlowercase() {}
func TestMain(m *testing.M) {}
func (s *Suite) TestMethod() {}
func FuzzInput(f *testing.F) {}
func BenchmarkSkip(b *testing.B) {}
func Example() {
    // Output: ok
}
func ExampleEmpty() {
    // Output:
}
func ExampleDocumentationOnly() {}
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	names, err := testNames(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test", "Test_边界", "FuzzInput", "Example", "ExampleEmpty"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}

func TestIntegrationPlanRejectsUnreadableOrInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{"missing_test.go", "invalid_test.go"} {
		if file == "invalid_test.go" {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("not Go"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		_, err := integrationPlan(nil, []testPackage{{Dir: dir, ImportPath: "fixture", TestGoFiles: []string{file}}})
		if err == nil {
			t.Fatalf("expected %s to fail", file)
		}
	}
}
