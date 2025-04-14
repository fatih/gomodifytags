package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatih/gomodifytags/modifytags"
)

var update = flag.Bool("update", false, "update golden (.out) files")

// This is the directory where our test fixtures are.
const fixtureDir = "./test-fixtures"

func TestParseFlags(t *testing.T) {
	// don't output help message during the test
	flag.CommandLine.SetOutput(ioutil.Discard)

	// The flag.CommandLine.Parse() call fails if there are flags re-defined
	// with the same name. If there are duplicates, parseFlags() will return
	// an error.
	_, _, err := parseFlags([]string{"-struct", "Server", "-add-tags", "json,xml", "-transform", "invalid"})
	if err == nil || !reflect.DeepEqual("invalid transform value", err.Error()) {
		t.Fatal("expected error: " + err.Error())
	}
}

func TestRewrite(t *testing.T) {
	test := []struct {
		cfg  *config
		mod  *modifytags.Modification
		file string
		err  error
	}{
		{
			file: "struct_add",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "struct_add_underscore",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "struct_add_existing",

			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "struct_format",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:         []string{"gaum"},
				Transform:   modifytags.SnakeCase,
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_format_existing",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:         []string{"gaum"},
				Transform:   modifytags.SnakeCase,
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_format_oldstyle",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:         []string{"gaum"},
				Transform:   modifytags.SnakeCase,
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_format_existing_oldstyle",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:         []string{"gaum"},
				Transform:   modifytags.SnakeCase,
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_remove",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Remove: []string{"json"},
			},
		},
		{
			file: "struct_clear_tags",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Clear: true,
			},
		},
		{
			file: "struct_clear_options",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				ClearOptions: true,
			},
		},
		{
			file: "line_add",
			cfg: &config{
				output: "source",
				line:   "4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_override",
			cfg: &config{
				output: "source",
				line:   "4,5",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
				Overwrite: true,
			},
		},
		{
			file: "line_add_override_column",
			cfg: &config{
				output: "source",
				line:   "4,4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json:MyBar:bar"},
				Transform: modifytags.SnakeCase,
				Overwrite: true,
			},
		},
		{
			file: "line_add_override_mixed_column_and_equal",
			cfg: &config{
				output: "source",
				line:   "4,4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json:MyBar:bar:foo=qux"},
				Transform: modifytags.SnakeCase,
				Overwrite: true,
			},
		},
		{
			file: "line_add_override_multi_equal",
			cfg: &config{
				output: "source",
				line:   "4,4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json:MyBar=bar=foo"},
				Transform: modifytags.SnakeCase,
				Overwrite: true,
			},
		},
		{
			file: "line_add_override_multi_column",
			cfg: &config{
				output: "source",
				line:   "4,4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json:MyBar:bar:foo"},
				Transform: modifytags.SnakeCase,
				Overwrite: true,
			},
		},
		{
			file: "line_add_no_override",
			cfg: &config{
				output: "source",
				line:   "4,5",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_outside",
			cfg: &config{
				output: "source",
				line:   "2,8",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_outside_partial_start",
			cfg: &config{
				output: "source",
				line:   "2,5",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_outside_partial_end",
			cfg: &config{
				output: "source",
				line:   "5,8",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_intersect_partial",
			cfg: &config{
				output: "source",
				line:   "5,11",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_comment",
			cfg: &config{
				output: "source",
				line:   "6,7",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_option",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				AddOptions: map[string][]string{
					"json": {"omitempty"},
				},
			},
		},
		{
			file: "line_add_option_existing",
			cfg: &config{
				output: "source",
				line:   "6,8",
			},
			mod: &modifytags.Modification{
				AddOptions: map[string][]string{
					"json": {"omitempty"},
				},
			},
		},
		{
			file: "line_add_multiple_option",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				AddOptions: map[string][]string{
					"json": {"omitempty"},
					"hcl":  {"squash"},
				},
				Add:       []string{"hcl"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_add_option_with_equal",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				AddOptions: map[string][]string{
					"validate": {"max=32"},
				},
				Add:       []string{"validate"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_remove",
			cfg: &config{
				output: "source",
				line:   "5,7",
			},
			mod: &modifytags.Modification{
				Remove: []string{"json"},
			},
		},
		{
			file: "line_remove_option",
			cfg: &config{
				output: "source",
				line:   "4,8",
			},
			mod: &modifytags.Modification{
				RemoveOptions: map[string][]string{
					"hcl": {"squash"},
				},
			},
		},
		{
			file: "line_remove_options",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				RemoveOptions: map[string][]string{
					"hcl":  {"omitnested"},
					"json": {"omitempty"},
				},
			},
		},
		{
			file: "line_remove_option_with_equal",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				RemoveOptions: map[string][]string{
					"validate": {"max=32"},
				},
			},
		},
		{
			file: "line_multiple_add",
			cfg: &config{
				output: "source",
				line:   "5,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "line_lispcase_add",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.LispCase,
			},
		},
		{
			file: "line_camelcase_add",
			cfg: &config{
				output: "source",
				line:   "4,5",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "line_camelcase_add_embedded",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "line_value_add",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add: []string{"json:foo"},
			},
		},
		{
			file: "offset_add",
			cfg: &config{
				output: "source",
				offset: 32,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "offset_add_composite",
			cfg: &config{
				output: "source",
				offset: 40,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "offset_add_duplicate",
			cfg: &config{
				output: "source",
				offset: 209,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "offset_add_literal_in",
			cfg: &config{
				output: "source",
				offset: 46,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "offset_add_literal_out",
			cfg: &config{
				output: "source",
				offset: 32,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "errors",
			cfg: &config{
				output: "source",
				line:   "4,7",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_pascalcase_add",
			cfg: &config{
				output: "source",
				line:   "4,5",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.PascalCase,
			},
		},
		{
			file: "line_pascalcase_add_embedded",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.PascalCase,
			},
		},
		{
			file: "not_formatted",
			cfg: &config{
				output: "source",
				line:   "3,4",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "skip_private",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:                  []string{"json"},
				Transform:            modifytags.SnakeCase,
				SkipUnexportedFields: true,
			},
		},
		{
			file: "skip_private_multiple_names",
			cfg: &config{
				output:     "source",
				structName: "foo",
			},
			mod: &modifytags.Modification{
				Add:                  []string{"json"},
				Transform:            modifytags.SnakeCase,
				SkipUnexportedFields: true,
			},
		},
		{
			file: "skip_embedded",
			cfg: &config{
				output:     "source",
				structName: "StationCreated",
			},
			mod: &modifytags.Modification{
				Add:                  []string{"json"},
				Transform:            modifytags.SnakeCase,
				SkipUnexportedFields: true,
			},
		},
		{
			file: "all_structs",
			cfg: &config{
				output: "source",
				all:    true,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "line_titlecase_add",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.TitleCase,
			},
		},
		{
			file: "line_titlecase_add_embedded",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.TitleCase,
			},
		},
		{
			file: "field_add",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "bar",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "field_add_same_line",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "qux",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "field_add_existing",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "bar",
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.SnakeCase,
			},
		},
		{
			file: "field_clear_tags",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "bar",
			},
			mod: &modifytags.Modification{
				Clear: true,
			},
		},
		{
			file: "field_clear_options",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "bar",
			},
			mod: &modifytags.Modification{
				ClearOptions: true,
			},
		},
		{
			file: "field_remove",
			cfg: &config{
				output:     "source",
				structName: "foo",
				fieldName:  "bar",
			},
			mod: &modifytags.Modification{
				Remove: []string{"json"},
			},
		},
		{
			file: "offset_anonymous_struct",
			cfg: &config{
				output: "source",
				offset: 45,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "offset_star_struct",
			cfg: &config{
				output: "source",
				offset: 35,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "offset_array_struct",
			cfg: &config{
				output: "source",
				offset: 35,
			},
			mod: &modifytags.Modification{
				Add:       []string{"json"},
				Transform: modifytags.CamelCase,
			},
		},
		{
			file: "empty_file",
			cfg: &config{
				output: "source",
				all:    true,
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "empty_file",
			cfg: &config{
				output: "source",
				line:   "4,6",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
			err: errors.New("line selection \"4,6\" is invalid: 4 is not within [0, 1]"),
		},
	}

	for _, ts := range test {
		t.Run(ts.file, func(t *testing.T) {
			ts.cfg.file = filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file))

			node, err := ts.cfg.parse()
			if err != nil {
				t.Fatal(err)
			}

			start, end, err := ts.cfg.findSelection(node)
			if err != nil {
				if !reflect.DeepEqual(ts.err.Error(), err.Error()) {
					t.Fatal(err)
				}
				return
			}

			ts.cfg.start = ts.cfg.fset.Position(start).Line
			ts.cfg.end = ts.cfg.fset.Position(end).Line

			err = ts.mod.Apply(ts.cfg.fset, node, start, end)
			if err != nil {
				if _, ok := err.(*modifytags.RewriteErrors); !ok {
					t.Fatal(err)
				}
			}

			var out string
			if err != nil {
				out, err = ts.cfg.format(node, err.(*modifytags.RewriteErrors))
			} else {
				out, err = ts.cfg.format(node, nil)
			}

			if err != nil {
				t.Fatal(err)
			}
			got := []byte(out)

			// update golden file if necessary
			golden := filepath.Join(fixtureDir, fmt.Sprintf("%s.golden", ts.file))
			if *update {
				err := ioutil.WriteFile(golden, got, 0644)
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

			var from []byte
			if ts.cfg.modified != nil {
				from, err = ioutil.ReadAll(ts.cfg.modified)
			} else {
				from, err = ioutil.ReadFile(ts.cfg.file)
			}
			if err != nil {
				t.Fatal(err)
			}

			// compare
			if !bytes.Equal(got, want) {
				t.Errorf("case %s\ngot:\n====\n\n%s\nwant:\n=====\n\n%s\nfrom:\n=====\n\n%s\n",
					ts.file, got, want, from)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	test := []struct {
		cfg  *config
		mod  *modifytags.Modification
		file string
		err  error
	}{
		{
			file: "json_single",
			cfg: &config{
				line: "5",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_full",
			cfg: &config{
				line: "4,6",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_intersection",
			cfg: &config{
				line: "5,16",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			// both small & end range larger than file
			file: "json_single",
			cfg: &config{
				line: "30,32", //invalid selection
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
			err: errors.New("line selection \"30,32\" is invalid: 30 is not within [0, 22]"),
		},
		{
			// end range larger than file
			file: "json_single",
			cfg: &config{
				line: "4,50", //invalid selection
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
			err: errors.New("line selection \"4,50\" is invalid: 4 is not within [0, 22]"),
		},
		{
			file: "json_errors",
			cfg: &config{
				line: "4,7",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted",
			cfg: &config{
				line: "3,4",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted_2",
			cfg: &config{
				line: "3,3",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted_3",
			cfg: &config{
				offset: 23,
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted_4",
			cfg: &config{
				offset: 51,
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted_5",
			cfg: &config{
				offset: 29,
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_not_formatted_6",
			cfg: &config{
				line: "2,54",
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
		{
			file: "json_all_structs",
			cfg: &config{
				all: true,
			},
			mod: &modifytags.Modification{
				Add: []string{"json"},
			},
		},
	}

	for _, ts := range test {
		t.Run(ts.file, func(t *testing.T) {
			ts.cfg.file = filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file))
			// these are explicit and shouldn't be changed for this particular
			// main test
			ts.cfg.output = "json"
			ts.mod.Transform = modifytags.CamelCase

			node, err := ts.cfg.parse()
			if err != nil {
				t.Fatal(err)
			}

			start, end, err := ts.cfg.findSelection(node)

			ts.cfg.start = ts.cfg.fset.Position(start).Line
			ts.cfg.end = ts.cfg.fset.Position(end).Line

			if err != nil {
				if !reflect.DeepEqual(ts.err.Error(), err.Error()) {
					t.Fatal(err)
				}
				return
			}

			err = ts.mod.Apply(ts.cfg.fset, node, start, end)
			if err != nil {
				if _, ok := err.(*modifytags.RewriteErrors); !ok {
					t.Fatal(err)
				}
			}

			var out string
			if err != nil {
				out, err = ts.cfg.format(node, err.(*modifytags.RewriteErrors))
			} else {
				out, err = ts.cfg.format(node, nil)
			}

			if err != nil && ts.err != nil && !reflect.DeepEqual(err.Error(), ts.err.Error()) {
				t.Logf("want: %v", ts.err)
				t.Logf("got: %v", err)
				t.Fatalf("unexpected error")
			}

			if ts.err != nil {
				return
			}

			got := []byte(out)

			// update golden file if necessary
			golden := filepath.Join(fixtureDir, fmt.Sprintf("%s.golden", ts.file))
			if *update {
				err := ioutil.WriteFile(golden, got, 0644)
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

			from, err := ioutil.ReadFile(ts.cfg.file)
			if err != nil {
				t.Fatal(err)
			}

			// compare
			if !bytes.Equal(got, want) {
				t.Errorf("case %s\ngot:\n====\n\n%s\nwant:\n=====\n\n%s\nfrom:\n=====\n\n%s\n",
					ts.file, got, want, from)
			}
		})
	}
}

func TestModifiedRewrite(t *testing.T) {
	cfg := &config{
		output:     "source",
		structName: "foo",
		file:       "struct_add_modified",
		modified: strings.NewReader(`struct_add_modified
55
package foo

type foo struct {
	bar string
	t   bool
}
`),
	}

	mod := &modifytags.Modification{
		Add:       []string{"json"},
		Transform: modifytags.SnakeCase,
	}

	node, err := cfg.parse()
	if err != nil {
		t.Fatal(err)
	}

	start, end, err := cfg.findSelection(node)
	if err != nil {
		t.Fatal(err)
	}

	cfg.start = cfg.fset.Position(start).Line
	cfg.end = cfg.fset.Position(end).Line

	err = mod.Apply(cfg.fset, node, start, end)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	if err != nil {
		got, err = cfg.format(node, err.(*modifytags.RewriteErrors))
	} else {
		got, err = cfg.format(node, nil)
	}

	if err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join(fixtureDir, "struct_add.golden")
	want, err := ioutil.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}

	// compare
	if !bytes.Equal([]byte(got), want) {
		t.Errorf("got:\n====\n%s\nwant:\n====\n%s\n", got, want)
	}
}

func TestModifiedFileMissing(t *testing.T) {
	cfg := &config{
		output:     "source",
		structName: "foo",
		file:       "struct_add_modified",
		modified: strings.NewReader(`file_that_doesnt_exist
55
package foo

type foo struct {
	bar string
	t   bool
}
`),
	}

	_, err := cfg.parse()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseLines(t *testing.T) {
	var tests = []struct {
		file string
	}{
		{file: "line_directive_unix"},
		{file: "line_directive_windows"},
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

			out, err := parseLines(file)
			if err != nil {
				t.Fatal(err)
			}

			toBytes := func(Lines []string) []byte {
				var buf bytes.Buffer
				for _, Line := range Lines {
					buf.WriteString(Line + "\n")
				}
				return buf.Bytes()
			}

			got := toBytes(out)

			// update golden file if necessary
			golden := filepath.Join(fixtureDir, fmt.Sprintf("%s.golden", ts.file))

			if *update {
				err := ioutil.WriteFile(golden, got, 0644)
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
			if !bytes.Equal(got, want) {
				t.Errorf("case %s\ngot:\n====\n\n%s\nwant:\n=====\n\n%s\nfrom:\n=====\n\n%s\n",
					ts.file, got, want, from)
			}

		})
	}
}
