package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
)

var writeTxtarGolden = flag.Bool("write-txtar-golden", false, "If true, writes out golden files in txtar archives")
var forceLegacyPattern = flag.String("force-legacy-pattern", "", "If set, forces legacy mode for txtar files matching this regexp pattern")

// shouldRunTxtarFile determines if a txtar file should run based on mode and comment
func shouldRunTxtarFile(comment string, filename string) bool {
	hasLegacyCompat := strings.Contains(strings.ToLower(comment), "legacy-compat")

	// Check if this file matches the force legacy pattern
	isForceMatch := false
	if *forceLegacyPattern != "" {
		matched, err := regexp.MatchString(*forceLegacyPattern, filename)
		if err == nil && matched {
			isForceMatch = true
		}
	}

	isLegacy := legacyMode || isForceMatch

	if isLegacy {
		// In legacy mode, only run files with legacy-compat
		return hasLegacyCompat
	}

	// Default mode: run all files (both legacy-compat and non-legacy-compat)
	return true
}

func TestTxtarGenerate(t *testing.T) {
	// Look for txtar files in testdata and current directory
	txtarFiles, err := filepath.Glob("testdata/*.txtar")
	if err != nil {
		t.Fatalf("failed to find txtar files in testdata: %v", err)
	}

	moreTxtarFiles, err := filepath.Glob("*.txtar")
	if err != nil {
		t.Fatalf("failed to find txtar files in current dir: %v", err)
	}

	txtarFiles = append(txtarFiles, moreTxtarFiles...)

	if len(txtarFiles) == 0 {
		t.Skip("no txtar files found")
	}

	for _, txtarFile := range txtarFiles {
		// Check if this file should run in the current mode
		archive, err := txtar.ParseFile(txtarFile)
		if err != nil {
			t.Errorf("failed to parse txtar file %s for mode check: %v", txtarFile, err)
			continue
		}

		if !shouldRunTxtarFile(string(archive.Comment), filepath.Base(txtarFile)) {
			t.Logf("Skipping %s (mode filter)", filepath.Base(txtarFile))
			continue
		}

		t.Run(filepath.Base(txtarFile), func(t *testing.T) {
			runTxtarTest(t, txtarFile)
		})
	}
}

func runTxtarTest(t *testing.T, txtarFile string) {
	archive, err := txtar.ParseFile(txtarFile)
	if err != nil {
		t.Fatalf("failed to parse txtar file %s: %v", txtarFile, err)
	}

	// Group files by test case (based on prefix before first dot)
	testCases := make(map[string]struct {
		json        []byte
		golden      []byte
		expectedErr []byte
		name        string
	})

	for _, file := range archive.Files {
		name := file.Name
		if strings.HasSuffix(name, ".json") {
			testName := strings.TrimSuffix(name, ".json")
			tc := testCases[testName]
			tc.json = file.Data
			tc.name = testName
			testCases[testName] = tc
		} else if strings.HasSuffix(name, ".go") {
			testName := strings.TrimSuffix(name, ".go")
			tc := testCases[testName]
			tc.golden = file.Data
			tc.name = testName
			testCases[testName] = tc
		} else if strings.HasSuffix(name, ".err") {
			testName := strings.TrimSuffix(name, ".err")
			tc := testCases[testName]
			tc.expectedErr = file.Data
			tc.name = testName
			testCases[testName] = tc
		}
	}

	var modifiedArchive *txtar.Archive
	var needsUpdate bool

	for testName, tc := range testCases {
		t.Run(testName, func(t *testing.T) {
			if len(tc.json) == 0 {
				t.Skip("no JSON input found")
				return
			}

			g := &generator{
				TypeName:    testName,
				PackageName: "test_package",
				OmitEmpty:   true,
			}

			var buf bytes.Buffer
			err := g.generate(&buf, bytes.NewReader(tc.json))

			// Check if we expect an error
			if len(tc.expectedErr) > 0 {
				expectedErrStr := strings.TrimSpace(string(tc.expectedErr))
				if err == nil {
					t.Errorf("expected error containing %q, but got none", expectedErrStr)
					return
				}
				if !strings.Contains(err.Error(), expectedErrStr) {
					t.Errorf("expected error containing %q, got %q", expectedErrStr, err.Error())
				}
				t.Logf("generator.generate() got expected error = %v", err)
				return
			}

			// If no error expected, but we got one
			if err != nil {
				if *writeTxtarGolden {
					// Write error expectation file
					if modifiedArchive == nil {
						modifiedArchive = &txtar.Archive{
							Comment: archive.Comment,
							Files:   make([]txtar.File, len(archive.Files)),
						}
						copy(modifiedArchive.Files, archive.Files)
					}

					// Find and update the corresponding .err file
					errFileName := testName + ".err"
					found := false
					for i, file := range modifiedArchive.Files {
						if file.Name == errFileName {
							modifiedArchive.Files[i].Data = []byte(err.Error())
							found = true
							needsUpdate = true
							break
						}
					}

					// If not found, append new error file
					if !found {
						modifiedArchive.Files = append(modifiedArchive.Files, txtar.File{
							Name: errFileName,
							Data: []byte(err.Error()),
						})
						needsUpdate = true
					}

					t.Logf("wrote error expectation for %s: %v", testName, err)
					return
				}
				t.Errorf("generator.generate() error = %v", err)
				return
			}

			got := buf.String()

			if *writeTxtarGolden {
				// Update the golden file in the archive
				if modifiedArchive == nil {
					modifiedArchive = &txtar.Archive{
						Comment: archive.Comment,
						Files:   make([]txtar.File, len(archive.Files)),
					}
					copy(modifiedArchive.Files, archive.Files)
				}

				// Find and update the corresponding .go file
				goldenFileName := testName + ".go"
				found := false
				for i, file := range modifiedArchive.Files {
					if file.Name == goldenFileName {
						modifiedArchive.Files[i].Data = []byte(got)
						found = true
						needsUpdate = true
						break
					}
				}

				// If not found, append new golden file
				if !found {
					modifiedArchive.Files = append(modifiedArchive.Files, txtar.File{
						Name: goldenFileName,
						Data: []byte(got),
					})
					needsUpdate = true
				}

				t.Logf("updated golden file for %s in txtar archive", testName)
				return
			}

			if len(tc.golden) == 0 {
				t.Logf("no golden file found for %s, generated:\n%s", testName, got)
				return
			}

			want := string(tc.golden)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("generate() mismatch for %s (-want +got):\n%s", testName, diff)
			}
		})
	}

	// Write updated txtar file if golden files were updated
	if *writeTxtarGolden && needsUpdate && modifiedArchive != nil {
		data := txtar.Format(modifiedArchive)
		err := os.WriteFile(txtarFile, data, 0644)
		if err != nil {
			t.Errorf("failed to write updated txtar file %s: %v", txtarFile, err)
		} else {
			t.Logf("wrote updated txtar file: %s", txtarFile)
		}
	}
}
