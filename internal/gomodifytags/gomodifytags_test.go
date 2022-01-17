package gomodifytags

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
)

var update = flag.Bool("update", false, "update golden (.out) files")

// This is the directory where our test fixtures are.
const fixtureDir = "./test-fixtures"

func TestRewrite(t *testing.T) {
	test := []struct {
		cfg  *Config
		file string
	}{
		{
			file: "struct_add",
			cfg: &Config{
				Add:        []string{"json"},
				Output:     "source",
				StructName: "foo",
				Transform:  "snakecase",
			},
		},
		{
			file: "struct_add_existing",
			cfg: &Config{
				Add:        []string{"json"},
				Output:     "source",
				StructName: "foo",
				Transform:  "snakecase",
			},
		},
		{
			file: "struct_format",
			cfg: &Config{
				Add:         []string{"gaum"},
				Output:      "source",
				StructName:  "foo",
				Transform:   "snakecase",
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_format_existing",
			cfg: &Config{
				Add:         []string{"gaum"},
				Output:      "source",
				StructName:  "foo",
				Transform:   "snakecase",
				ValueFormat: "field_name={field}",
			},
		},
		{
			file: "struct_format_oldstyle",
			cfg: &Config{
				Add:         []string{"gaum"},
				Output:      "source",
				StructName:  "foo",
				Transform:   "snakecase",
				ValueFormat: "field_name=$field",
			},
		},
		{
			file: "struct_format_existing_oldstyle",
			cfg: &Config{
				Add:         []string{"gaum"},
				Output:      "source",
				StructName:  "foo",
				Transform:   "snakecase",
				ValueFormat: "field_name=$field",
			},
		},
		{
			file: "struct_remove",
			cfg: &Config{
				Remove:     []string{"json"},
				Output:     "source",
				StructName: "foo",
			},
		},
		{
			file: "struct_clear_tags",
			cfg: &Config{
				Clear:      true,
				Output:     "source",
				StructName: "foo",
			},
		},
		{
			file: "struct_clear_options",
			cfg: &Config{
				ClearOption: true,
				Output:      "source",
				StructName:  "foo",
			},
		},
		{
			file: "line_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_override",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,5",
				Transform: "snakecase",
				Override:  true,
			},
		},
		{
			file: "line_add_override_column",
			cfg: &Config{
				Add:       []string{"json:MyBar:bar"},
				Output:    "source",
				Line:      "4,4",
				Transform: "snakecase",
				Override:  true,
			},
		},
		{
			file: "line_add_override_mixed_column_and_equal",
			cfg: &Config{
				Add:       []string{"json:MyBar:bar:foo=qux"},
				Output:    "source",
				Line:      "4,4",
				Transform: "snakecase",
				Override:  true,
			},
		},
		{
			file: "line_add_override_multi_equal",
			cfg: &Config{
				Add:       []string{"json:MyBar=bar=foo"},
				Output:    "source",
				Line:      "4,4",
				Transform: "snakecase",
				Override:  true,
			},
		},
		{
			file: "line_add_override_multi_column",
			cfg: &Config{
				Add:       []string{"json:MyBar:bar:foo"},
				Output:    "source",
				Line:      "4,4",
				Transform: "snakecase",
				Override:  true,
			},
		},
		{
			file: "line_add_no_override",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,5",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_outside",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "2,8",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_outside_partial_start",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "2,5",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_outside_partial_end",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "5,8",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_intersect_partial",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "5,11",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_comment",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "6,7",
				Transform: "snakecase",
			},
		},
		{
			file: "line_add_option",
			cfg: &Config{
				AddOptions: []string{"json=omitempty"},
				Output:     "source",
				Line:       "4,7",
			},
		},
		{
			file: "line_add_option_existing",
			cfg: &Config{
				AddOptions: []string{"json=omitempty"},
				Output:     "source",
				Line:       "6,8",
			},
		},
		{
			file: "line_add_multiple_option",
			cfg: &Config{
				AddOptions: []string{"json=omitempty", "hcl=squash"},
				Add:        []string{"hcl"},
				Output:     "source",
				Line:       "4,7",
				Transform:  "snakecase",
			},
		},
		{
			file: "line_add_option_with_equal",
			cfg: &Config{
				AddOptions: []string{"validate=max=32"},
				Add:        []string{"validate"},
				Output:     "source",
				Line:       "4,7",
				Transform:  "snakecase",
			},
		},
		{
			file: "line_remove",
			cfg: &Config{
				Remove: []string{"json"},
				Output: "source",
				Line:   "5,7",
			},
		},
		{
			file: "line_remove_option",
			cfg: &Config{
				RemoveOptions: []string{"hcl=squash"},
				Output:        "source",
				Line:          "4,8",
			},
		},
		{
			file: "line_remove_options",
			cfg: &Config{
				RemoveOptions: []string{"json=omitempty", "hcl=omitnested"},
				Output:        "source",
				Line:          "4,7",
			},
		},
		{
			file: "line_remove_option_with_equal",
			cfg: &Config{
				RemoveOptions: []string{"validate=max=32"},
				Output:        "source",
				Line:          "4,7",
			},
		},
		{
			file: "line_multiple_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "5,6",
				Transform: "camelcase",
			},
		},
		{
			file: "line_lispcase_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,6",
				Transform: "lispcase",
			},
		},
		{
			file: "line_camelcase_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,5",
				Transform: "camelcase",
			},
		},
		{
			file: "line_camelcase_add_embedded",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,6",
				Transform: "camelcase",
			},
		},
		{
			file: "line_value_add",
			cfg: &Config{
				Add:    []string{"json:foo"},
				Output: "source",
				Line:   "4,6",
			},
		},
		{
			file: "offset_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    32,
				Transform: "snakecase",
			},
		},
		{
			file: "offset_add_composite",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    40,
				Transform: "snakecase",
			},
		},
		{
			file: "offset_add_duplicate",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    209,
				Transform: "snakecase",
			},
		},
		{
			file: "offset_add_literal_in",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    46,
				Transform: "snakecase",
			},
		},
		{
			file: "offset_add_literal_out",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    32,
				Transform: "snakecase",
			},
		},
		{
			file: "errors",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,7",
				Transform: "snakecase",
			},
		},
		{
			file: "line_pascalcase_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,5",
				Transform: "pascalcase",
			},
		},
		{
			file: "line_pascalcase_add_embedded",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,6",
				Transform: "pascalcase",
			},
		},
		{
			file: "not_formatted",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "3,4",
				Transform: "snakecase",
			},
		},
		{
			file: "skip_private",
			cfg: &Config{
				Add:                  []string{"json"},
				Output:               "source",
				StructName:           "foo",
				Transform:            "snakecase",
				SkipUnexportedFields: true,
			},
		},
		{
			file: "skip_private_multiple_names",
			cfg: &Config{
				Add:                  []string{"json"},
				Output:               "source",
				StructName:           "foo",
				Transform:            "snakecase",
				SkipUnexportedFields: true,
			},
		},
		{
			file: "skip_embedded",
			cfg: &Config{
				Add:                  []string{"json"},
				Output:               "source",
				StructName:           "StationCreated",
				Transform:            "snakecase",
				SkipUnexportedFields: true,
			},
		},
		{
			file: "all_structs",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				All:       true,
				Transform: "snakecase",
			},
		},
		{
			file: "line_titlecase_add",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,6",
				Transform: "titlecase",
			},
		},
		{
			file: "line_titlecase_add_embedded",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Line:      "4,6",
				Transform: "titlecase",
			},
		},
		{
			file: "field_add",
			cfg: &Config{
				Add:        []string{"json"},
				Output:     "source",
				StructName: "foo",
				FieldName:  "bar",
				Transform:  "snakecase",
			},
		},
		{
			file: "field_add_same_line",
			cfg: &Config{
				Add:        []string{"json"},
				Output:     "source",
				StructName: "foo",
				FieldName:  "qux",
				Transform:  "snakecase",
			},
		},
		{
			file: "field_add_existing",
			cfg: &Config{
				Add:        []string{"json"},
				Output:     "source",
				StructName: "foo",
				FieldName:  "bar",
				Transform:  "snakecase",
			},
		},
		{
			file: "field_clear_tags",
			cfg: &Config{
				Clear:      true,
				Output:     "source",
				StructName: "foo",
				FieldName:  "bar",
			},
		},
		{
			file: "field_clear_options",
			cfg: &Config{
				ClearOption: true,
				Output:      "source",
				StructName:  "foo",
				FieldName:   "bar",
			},
		},
		{
			file: "field_remove",
			cfg: &Config{
				Remove:     []string{"json"},
				Output:     "source",
				StructName: "foo",
				FieldName:  "bar",
			},
		},
		{
			file: "offset_anonymous_struct",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    45,
				Transform: "camelcase",
			},
		},
		{
			file: "offset_star_struct",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    35,
				Transform: "camelcase",
			},
		},
		{
			file: "offset_array_struct",
			cfg: &Config{
				Add:       []string{"json"},
				Output:    "source",
				Offset:    35,
				Transform: "camelcase",
			},
		},
	}

	for _, ts := range test {
		t.Run(ts.file, func(t *testing.T) {
			ts.cfg.File = filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file))

			node, err := ts.cfg.parse()
			if err != nil {
				t.Fatal(err)
			}

			start, end, err := ts.cfg.findSelection(node)
			if err != nil {
				t.Fatal(err)
			}

			rewrittenNode, err := ts.cfg.rewrite(node, start, end)
			if err != nil {
				if _, ok := err.(*rewriteErrors); !ok {
					t.Fatal(err)
				}
			}

			out, err := ts.cfg.format(rewrittenNode, err)
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
			if ts.cfg.Modified != nil {
				from, err = ioutil.ReadAll(ts.cfg.Modified)
			} else {
				from, err = ioutil.ReadFile(ts.cfg.File)
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
		cfg  *Config
		file string
		err  error
	}{
		{
			file: "json_single",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "5",
			},
		},
		{
			file: "json_full",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "4,6",
			},
		},
		{
			file: "json_intersection",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "5,16",
			},
		},
		{
			// both small & end range larger than file
			file: "json_single",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "30,32", // invalid selection
			},
			err: errors.New("line selection is invalid"),
		},
		{
			// end range larger than file
			file: "json_single",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "4,50", // invalid selection
			},
			err: errors.New("line selection is invalid"),
		},
		{
			file: "json_errors",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "4,7",
			},
		},
		{
			file: "json_not_formatted",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "3,4",
			},
		},
		{
			file: "json_not_formatted_2",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "3,3",
			},
		},
		{
			file: "json_not_formatted_3",
			cfg: &Config{
				Add:    []string{"json"},
				Offset: 23,
			},
		},
		{
			file: "json_not_formatted_4",
			cfg: &Config{
				Add:    []string{"json"},
				Offset: 51,
			},
		},
		{
			file: "json_not_formatted_5",
			cfg: &Config{
				Add:    []string{"json"},
				Offset: 29,
			},
		},
		{
			file: "json_not_formatted_6",
			cfg: &Config{
				Add:  []string{"json"},
				Line: "2,54",
			},
		},
		{
			file: "json_all_structs",
			cfg: &Config{
				Add: []string{"json"},
				All: true,
			},
		},
	}

	for _, ts := range test {
		t.Run(ts.file, func(t *testing.T) {
			ts.cfg.File = filepath.Join(fixtureDir, fmt.Sprintf("%s.input", ts.file))
			// these are explicit and shouldn't be changed for this particular
			// main test
			ts.cfg.Output = "json"
			ts.cfg.Transform = "camelcase"

			node, err := ts.cfg.parse()
			if err != nil {
				t.Fatal(err)
			}

			start, end, err := ts.cfg.findSelection(node)
			if err != nil {
				t.Fatal(err)
			}

			rewrittenNode, err := ts.cfg.rewrite(node, start, end)
			if err != nil {
				if _, ok := err.(*rewriteErrors); !ok {
					t.Fatal(err)
				}
			}

			out, err := ts.cfg.format(rewrittenNode, err)
			if !reflect.DeepEqual(err, ts.err) {
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

			from, err := ioutil.ReadFile(ts.cfg.File)
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
	cfg := &Config{
		Add:        []string{"json"},
		Output:     "source",
		StructName: "foo",
		Transform:  "snakecase",
		File:       "struct_add_modified",
		Modified: strings.NewReader(`struct_add_modified
55
package foo

type foo struct {
	bar string
	t   bool
}
`),
	}

	node, err := cfg.parse()
	if err != nil {
		t.Fatal(err)
	}

	start, end, err := cfg.findSelection(node)
	if err != nil {
		t.Fatal(err)
	}

	rewrittenNode, err := cfg.rewrite(node, start, end)
	if err != nil {
		t.Fatal(err)
	}

	got, err := cfg.format(rewrittenNode, err)
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
	cfg := &Config{
		Add:        []string{"json"},
		Output:     "source",
		StructName: "foo",
		Transform:  "snakecase",
		File:       "struct_add_modified",
		Modified: strings.NewReader(`file_that_doesnt_exist
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

			toBytes := func(lines []string) []byte {
				var buf bytes.Buffer
				for _, line := range lines {
					buf.WriteString(line + "\n")
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
