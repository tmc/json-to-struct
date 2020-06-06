package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// If `-write-golden` is provided when running tests, golden files are written.
var writeGolden = false

func TestMain(m *testing.M) {
	var flagWriteGolden = flag.Bool("write-golden", false, "If true, writes out golden files")
	flag.Parse()
	if *flagWriteGolden {
		writeGolden = true
	}
	os.Exit(m.Run())
}

func TestGenerate(t *testing.T) {
	// t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "empty", wantErr: true},
		{name: "test_simple_json"},
		{name: "test_nested_json"},
		{name: "test_nullable_json"},
		{name: "test_repeated_json"},
		{name: "test_simple_array"},
		{name: "test_invalid_field_chars"},
		{name: "more_complex_example"},
	}
	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			//t.Parallel()
			input := openTestData(t, tt.name+".json")
			got, err := generate(bytes.NewReader(input), tt.name, "test_package", nil)
			if err != nil {
				if tt.wantErr {
					t.Logf("generate() got expected error = %v", err)
					return
				}
				t.Errorf("generate() error = %v, wantErr %v", err, tt.wantErr)
				t.Errorf("generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			goldenFile := tt.name + ".go"
			if writeGolden {
				t.Log("writing golden file")
				writeTestData(t, goldenFile, got)
				return
			}
			want := string(openTestData(t, goldenFile))
			gotStr := string(got)
			if diff := cmp.Diff(want, gotStr); diff != "" {
				t.Errorf("generate() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func openTestData(t *testing.T, filename string) []byte {
	input, err := ioutil.ReadFile("testdata/" + filename)
	if err != nil {
		t.Error(err)
	}
	return input
}

func writeTestData(t *testing.T, filename string, contents []byte) {
	err := ioutil.WriteFile("testdata/"+filename, contents, 0644)
	if err != nil {
		t.Error(err)
	}
}
func trimStringSlice(strs []string) []string {
	var result []string
	for _, s := range strs {
		n := strings.TrimSpace(s)
		result = append(result, n)
	}
	return result
}
