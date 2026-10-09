package modifytags

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden (.out) files")

// This is the directory where our test fixtures are.
const fixtureDir = "./test-fixtures"

func TestApply(t *testing.T) {
	var tests = []struct {
		file  string
		m     *Modification
		start token.Pos
		end   token.Pos
	}{
		{
			file: "all_structs",
			m: &Modification{
				Add: []string{"json"},
			},
			start: token.NoPos, // will be set to start of file
			end:   token.NoPos, // will be set to end of file
		},
		{
			file: "clear_all_tags",
			m: &Modification{
				Clear: true,
			},
			start: token.NoPos, // will be set to start of file
			end:   token.NoPos, // will be set to end of file
		},
		{
			file: "remove_some_tags",
			m: &Modification{
				Remove: []string{"json"},
			},
			start: token.NoPos, // will be set to start of file
			end:   token.NoPos, // will be set to end of file
		},
		{
			file: "add_tags_pos_between_line",
			m: &Modification{
				Add: []string{"json"},
				AddOptions: map[string][]string{
					"json": {"omitempty"},
				},
			},
			// TODO: use markers in the test content to delineate start and end, rather than hard-coding here.
			start: token.Pos(50),  // middle of second struct field
			end:   token.Pos(200), // middle of fourth struct field
		},
		{
			file: "lowercase",
			m: &Modification{
				Add:       []string{"json"},
				Transform: LowerCase,
			},
			start: token.NoPos,
			end:   token.NoPos,
		},
	}

	for _, ts := range tests {
		ts := ts

		t.Run(ts.file, func(t *testing.T) {
			filePath := filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file))
			file, err := os.Open(filePath)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()

			fset := token.NewFileSet()

			node, err := parser.ParseFile(fset, filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file)), nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			if ts.start == token.NoPos {
				ts.start = node.Pos()
			}
			if ts.end == token.NoPos {
				ts.end = node.End()
			}

			err = ts.m.Apply(fset, node, ts.start, ts.end)
			if err != nil {
				t.Fatal(err)
			}

			var got bytes.Buffer
			err = format.Node(&got, fset, node)
			if err != nil {
				t.Fatal(err)
			}

			// update golden file if necessary
			golden := filepath.Join(fixtureDir, fmt.Sprintf("%s.golden", ts.file))

			if *update {
				err := ioutil.WriteFile(golden, got.Bytes(), 0644)
				if err != nil {
					t.Error(err)
				}
				return
			}

			// get golden file
			want, err := ioutil.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}

			from, err := ioutil.ReadFile(filePath)
			if err != nil {
				t.Fatal(err)
			}

			// compare
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("case %s\ngot:\n====\n\n%s\nwant:\n=====\n\n%s\nfrom:\n=====\n\n%s\n",
					ts.file, got.Bytes(), want, from)
			}

		})
	}
}

func TestProcessField_LowerCase(t *testing.T) {
	tests := []struct {
		fieldName string
		want      string
	}{
		{fieldName: "bar", want: "`json:\"bar\"`"},
		{fieldName: "MyExample", want: "`json:\"myexample\"`"},
		{fieldName: "CreatedAt", want: "`json:\"createdat\"`"},
		{fieldName: "BaseURL", want: "`json:\"baseurl\"`"},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			m := &Modification{
				Add:       []string{"json"},
				Transform: LowerCase,
			}
			got, err := m.processField(tt.fieldName, "")
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != tt.want {
				t.Errorf("processField(%q) = %q, want %q", tt.fieldName, got, tt.want)
			}
		})
	}
}

