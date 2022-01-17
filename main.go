package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/gomodifytags/internal/gomodifytags"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func realMain() error {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	return cfg.Run()
}

func parseConfig(args []string) (*gomodifytags.Config, error) {
	var (
		// file flags
		flagFile  = flag.String("file", "", "Filename to be parsed")
		flagWrite = flag.Bool("w", false, "Write results to (source) file")
		flagQuiet = flag.Bool("quiet", false, "Don't print result to stdout")

		flagOutput = flag.String("format", "source", "Output format."+
			"By default it's the whole file. Options: [source, json]")
		flagModified = flag.Bool("modified", false, "read an archive of modified files from standard input")

		// processing modes
		flagOffset = flag.Int("offset", 0,
			"Byte offset of the cursor position inside a struct."+
				"Can be anwhere from the comment until closing bracket")
		flagLine = flag.String("line", "",
			"Line number of the field or a range of line. i.e: 4 or 4,8")
		flagStruct = flag.String("struct", "", "Struct name to be processed")
		flagField  = flag.String("field", "", "Field name to be processed")
		flagAll    = flag.Bool("all", false, "Select all structs to be processed")

		// tag flags
		flagRemoveTags = flag.String("remove-tags", "",
			"Remove tags for the comma separated list of keys")
		flagClearTags = flag.Bool("clear-tags", false,
			"Clear all tags")
		flagAddTags = flag.String("add-tags", "",
			"Adds tags for the comma separated list of keys."+
				"Keys can contain a static value, i,e: json:foo")
		flagOverride             = flag.Bool("override", false, "Override current tags when adding tags")
		flagSkipUnexportedFields = flag.Bool("skip-unexported", false, "Skip unexported fields")
		flagTransform            = flag.String("transform", "snakecase",
			"Transform adds a transform rule when adding tags."+
				" Current options: [snakecase, camelcase, lispcase, pascalcase, titlecase, keep]")
		flagSort = flag.Bool("sort", false,
			"Sort sorts the tags in increasing order according to the key name")

		// formatting
		flagFormatting = flag.String("template", "",
			"Format the given tag's value. i.e: \"column:{field}\", \"field_name={field}\"")

		// option flags
		flagRemoveOptions = flag.String("remove-options", "",
			"Remove the comma separated list of options from the given keys, "+
				"i.e: json=omitempty,hcl=squash")
		flagClearOptions = flag.Bool("clear-options", false,
			"Clear all tag options")
		flagAddOptions = flag.String("add-options", "",
			"Add the options per given key. i.e: json=omitempty,hcl=squash")
	)

	// this fails if there are flags re-defined with the same name.
	if err := flag.CommandLine.Parse(args); err != nil {
		return nil, err
	}

	if flag.NFlag() == 0 {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return nil, flag.ErrHelp
	}

	cfg := &gomodifytags.Config{
		File:                 *flagFile,
		Line:                 *flagLine,
		StructName:           *flagStruct,
		FieldName:            *flagField,
		Offset:               *flagOffset,
		All:                  *flagAll,
		Output:               *flagOutput,
		Write:                *flagWrite,
		Quiet:                *flagQuiet,
		Clear:                *flagClearTags,
		ClearOption:          *flagClearOptions,
		Transform:            *flagTransform,
		Sort:                 *flagSort,
		ValueFormat:          *flagFormatting,
		Override:             *flagOverride,
		SkipUnexportedFields: *flagSkipUnexportedFields,
	}

	if *flagModified {
		cfg.Modified = os.Stdin
	}

	if *flagAddTags != "" {
		cfg.Add = strings.Split(*flagAddTags, ",")
	}

	if *flagAddOptions != "" {
		cfg.AddOptions = strings.Split(*flagAddOptions, ",")
	}

	if *flagRemoveTags != "" {
		cfg.Remove = strings.Split(*flagRemoveTags, ",")
	}

	if *flagRemoveOptions != "" {
		cfg.RemoveOptions = strings.Split(*flagRemoveOptions, ",")
	}

	return cfg, nil
}
