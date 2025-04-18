package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/gomodifytags/modifytags"
	"golang.org/x/tools/go/buildutil"
)

// structType contains a structType node and it's name. It's a convenient
// helper type, because *ast.StructType doesn't contain the name of the struct.
// Also, we want to be able to handle structs selected by their variable name,
// so we can't simply use a TypeSpec.
type structType struct {
	name string
	node *ast.StructType
}

// output is used usually by editors
type output struct {
	Start  int      `json:"start"`
	End    int      `json:"end"`
	Lines  []string `json:"lines"`
	Errors []string `json:"errors,omitempty"`
}

// A config defines how the input is parsed and formatted.
type config struct {
	file     string
	output   string
	quiet    bool
	write    bool
	modified io.Reader

	offset     int
	structName string
	fieldName  string
	line       string
	start, end int
	all        bool

	fset *token.FileSet
}

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func realMain() error {
	cfg, mod, err := parseFlags(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	return run(cfg, mod)
}

func parseFlags(args []string) (*config, *modifytags.Modification, error) {
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
		return nil, nil, err
	}

	if flag.NFlag() == 0 {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		return nil, nil, flag.ErrHelp
	}

	cfg := &config{
		file:       *flagFile,
		line:       *flagLine,
		structName: *flagStruct,
		fieldName:  *flagField,
		offset:     *flagOffset,
		all:        *flagAll,
		output:     *flagOutput,
		write:      *flagWrite,
		quiet:      *flagQuiet,
	}

	transform, err := parseTransform(*flagTransform)
	if err != nil {
		return nil, nil, err
	}

	mod := &modifytags.Modification{
		Clear:                *flagClearTags,
		ClearOptions:         *flagClearOptions,
		Transform:            transform,
		Sort:                 *flagSort,
		ValueFormat:          *flagFormatting,
		Overwrite:            *flagOverride,
		SkipUnexportedFields: *flagSkipUnexportedFields,
	}

	if *flagModified {
		cfg.modified = os.Stdin
	}

	if *flagAddTags != "" {
		mod.Add = strings.Split(*flagAddTags, ",")
	}

	if *flagAddOptions != "" {
		parsedOptions, err := parseOptions(*flagAddOptions)
		if err != nil {
			return nil, nil, err
		}
		mod.AddOptions = parsedOptions
	}

	if *flagRemoveTags != "" {
		mod.Remove = strings.Split(*flagRemoveTags, ",")
	}

	if *flagRemoveOptions != "" {
		parsedOptions, err := parseOptions(*flagRemoveOptions)
		if err != nil {
			return nil, nil, err
		}
		mod.RemoveOptions = parsedOptions
	}

	return cfg, mod, nil
}

func run(cfg *config, mod *modifytags.Modification) error {
	err := cfg.validate()
	if err != nil {
		return err
	}

	file, err := cfg.parse()
	if err != nil {
		return err
	}

	start, end, err := cfg.findSelection(file)
	if err != nil {
		return err
	}
	cfg.start = cfg.fset.Position(start).Line
	cfg.end = cfg.fset.Position(end).Line

	errs := mod.Apply(cfg.fset, file, start, end)
	if errs != nil {
		if _, ok := errs.(*modifytags.RewriteErrors); !ok {
			return errs
		}
	}
	var out string
	if errs != nil {
		out, err = cfg.format(file, errs.(*modifytags.RewriteErrors))
	} else {
		out, err = cfg.format(file, nil)
	}

	if err != nil {
		return err
	}

	if !cfg.quiet {
		fmt.Println(out)
	}
	return nil
}

func (cfg *config) parse() (*ast.File, error) {
	cfg.fset = token.NewFileSet()
	var contents interface{}
	if cfg.modified != nil {
		archive, err := buildutil.ParseOverlayArchive(cfg.modified)
		if err != nil {
			return nil, fmt.Errorf("failed to parse -modified archive: %v", err)
		}
		fc, ok := archive[cfg.file]
		if !ok {
			return nil, fmt.Errorf("couldn't find %s in archive", cfg.file)
		}
		contents = fc
	}

	return parser.ParseFile(cfg.fset, cfg.file, contents, parser.ParseComments)
}

func parseTransform(input string) (modifytags.Transform, error) {
	input = strings.ToLower(input)
	switch input {
	case "camelcase":
		return modifytags.CamelCase, nil
	case "lispcase":
		return modifytags.LispCase, nil
	case "pascalcase":
		return modifytags.PascalCase, nil
	case "titlecase":
		return modifytags.TitleCase, nil
	case "keep":
		return modifytags.Keep, nil
	case "snakecase":
		return modifytags.SnakeCase, nil
	default:
		return modifytags.SnakeCase, fmt.Errorf("invalid transform value")
	}
}

func parseOptions(options string) (map[string][]string, error) {
	optionsMap := make(map[string][]string)
	list := strings.Split(options, ",")
	for _, item := range list {
		key, option, found := strings.Cut(item, "=")
		if !found {
			return nil, fmt.Errorf("invalid option %q; should be key=option", item)
		}
		optionsMap[key] = append(optionsMap[key], option)
	}
	return optionsMap, nil
}

// findSelection returns the start and end positions of the fields that are
// subject to change. It depends on the line, struct or offset selection.
func (cfg *config) findSelection(file *ast.File) (token.Pos, token.Pos, error) {
	switch {
	case cfg.line != "":
		return cfg.lineSelection(file)
	case cfg.offset != 0:
		return cfg.offsetSelection(file)
	case cfg.structName != "":
		return cfg.structSelection(file)
	case cfg.all:
		return cfg.allSelection(file)
	default:
		return 0, 0, errors.New("-line, -offset, -struct or -all is not passed")
	}
}

// collectStructs collects and maps structType nodes to their positions
func collectStructs(node ast.Node) map[token.Pos]*structType {
	structs := make(map[token.Pos]*structType, 0)

	collectStructs := func(n ast.Node) bool {
		var t ast.Expr
		var structName string

		switch x := n.(type) {
		case *ast.TypeSpec:
			if x.Type == nil {
				return true

			}

			structName = x.Name.Name
			t = x.Type
		case *ast.CompositeLit:
			t = x.Type
		case *ast.ValueSpec:
			structName = x.Names[0].Name
			t = x.Type
		case *ast.Field:
			// this case also catches struct fields and the structName
			// therefore might contain the field name (which is wrong)
			// because `x.Type` in this case is not a *ast.StructType.
			//
			// We're OK with it, because, in our case *ast.Field represents
			// a parameter declaration, i.e:
			//
			//   func test(arg struct {
			//   	Field int
			//   }) {
			//   }
			//
			// and hence the struct name will be `arg`.
			if len(x.Names) != 0 {
				structName = x.Names[0].Name
			}
			t = x.Type
		}

		// if expression is in form "*T" or "[]T", dereference to check if "T"
		// contains a struct expression
		t = deref(t)

		x, ok := t.(*ast.StructType)
		if !ok {
			return true
		}

		structs[x.Pos()] = &structType{
			name: structName,
			node: x,
		}
		return true
	}

	ast.Inspect(node, collectStructs)
	return structs
}

func (cfg *config) format(file ast.Node, rwErr *modifytags.RewriteErrors) (string, error) {
	switch cfg.output {
	case "source":
		var buf bytes.Buffer
		err := format.Node(&buf, cfg.fset, file)
		if err != nil {
			return "", err
		}

		if cfg.write {
			err = ioutil.WriteFile(cfg.file, buf.Bytes(), 0)
			if err != nil {
				return "", err
			}

		}

		return buf.String(), nil
	case "json":
		// NOTE(arslan): print first the whole file and then cut out our
		// selection. The reason we don't directly print the struct is that the
		// printer is not capable of printing loosy comments, comments that are
		// not part of any field inside a struct. Those are part of *ast.File
		// and only printed inside a struct if we print the whole file. This
		// approach is the sanest and simplest way to get a struct printed
		// back. Second, our cursor might intersect two different structs with
		// other declarations in between them. Printing the file and cutting
		// the selection is the easier and simpler to do.
		var buf bytes.Buffer

		// this is the default config from `format.Node()`, but we add
		// `printer.SourcePos` to get the original source position of the
		// modified lines
		pcfg := printer.Config{Mode: printer.SourcePos | printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
		err := pcfg.Fprint(&buf, cfg.fset, file)
		if err != nil {
			return "", err
		}

		lines, err := parseLines(&buf)
		if err != nil {
			return "", err
		}

		// prevent selection to be larger than the actual number of lines
		if cfg.start > len(lines) || cfg.end > len(lines) {
			return "", errors.New("line selection is invalid")
		}

		out := &output{
			Start: cfg.start,
			End:   cfg.end,
			Lines: lines[cfg.start-1 : cfg.end],
		}

		if rwErr != nil {
			for _, err := range rwErr.Errs {
				out.Errors = append(out.Errors, err.Error())
			}
		}

		o, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return "", err
		}

		return string(o), nil
	default:
		return "", fmt.Errorf("unknown output mode: %s", cfg.output)
	}
}

func (cfg *config) lineSelection(file *ast.File) (token.Pos, token.Pos, error) {
	var err error
	split := strings.Split(cfg.line, ",")

	start, err := strconv.Atoi(split[0])
	if err != nil {
		return 0, 0, err
	}

	end := start
	if len(split) == 2 {
		end, err = strconv.Atoi(split[1])
		if err != nil {
			return 0, 0, err
		}
	}

	if start > end {
		return 0, 0, errors.New("wrong range. start line cannot be larger than end line")
	}

	// Convert start and end line numbers to token.Pos.
	tokFile := cfg.fset.File(file.FileStart)
	lineCount := tokFile.LineCount()
	if start < 0 || start > lineCount || end > lineCount {
		return 0, 0, fmt.Errorf("line selection %q is invalid: %d is not within [0, %d]", cfg.line, start, lineCount)
	}
	startPos := tokFile.LineStart(start)
	var endPos token.Pos
	if end == tokFile.LineCount() {
		endPos = tokFile.Pos(tokFile.Size())
	} else {
		// Get the position of the end of the line
		endPos = tokFile.LineStart(end+1) - 1
	}
	return startPos, endPos, nil
}

func (cfg *config) structSelection(file *ast.File) (token.Pos, token.Pos, error) {
	structs := collectStructs(file)

	var encStruct *ast.StructType
	for _, st := range structs {
		if st.name == cfg.structName {
			encStruct = st.node
		}
	}

	if encStruct == nil {
		return 0, 0, errors.New("struct name does not exist")
	}

	// if field name has been specified as well, only select the given field
	if cfg.fieldName != "" {
		return cfg.fieldSelection(encStruct)
	}
	return encStruct.Pos(), encStruct.End(), nil
}

func (cfg *config) fieldSelection(st *ast.StructType) (token.Pos, token.Pos, error) {
	var encField *ast.Field
	for _, f := range st.Fields.List {
		for _, field := range f.Names {
			if field.Name == cfg.fieldName {
				encField = f
			}
		}
	}

	if encField == nil {
		return 0, 0, fmt.Errorf("struct %q doesn't have field name %q",
			cfg.structName, cfg.fieldName)
	}
	return encField.Pos(), encField.End(), nil
}

func (cfg *config) offsetSelection(file *ast.File) (token.Pos, token.Pos, error) {
	structs := collectStructs(file)

	var encStruct *ast.StructType
	for _, st := range structs {
		structBegin := cfg.fset.Position(st.node.Pos()).Offset
		structEnd := cfg.fset.Position(st.node.End()).Offset

		if structBegin <= cfg.offset && cfg.offset <= structEnd {
			encStruct = st.node
			break
		}
	}

	if encStruct == nil {
		return 0, 0, errors.New("offset is not inside a struct")
	}

	// offset selects all fields
	return encStruct.Pos(), encStruct.End(), nil
}

// allSelection selects all structs inside a file
func (cfg *config) allSelection(file *ast.File) (token.Pos, token.Pos, error) {
	tokFile := cfg.fset.File(file.FileStart)
	return tokFile.LineStart(1), tokFile.Pos(tokFile.Size()), nil
}

// validate determines whether the config is valid or not
func (cfg *config) validate() error {
	if cfg.file == "" {
		return errors.New("no file is passed")
	}

	if cfg.line == "" && cfg.offset == 0 && cfg.structName == "" && !cfg.all {
		return errors.New("-line, -offset, -struct or -all is not passed")
	}

	if cfg.line != "" && cfg.offset != 0 ||
		cfg.line != "" && cfg.structName != "" ||
		cfg.offset != 0 && cfg.structName != "" {
		return errors.New("-line, -offset or -struct cannot be used together. pick one")
	}

	if cfg.fieldName != "" && cfg.structName == "" {
		return errors.New("-field is requiring -struct")
	}

	return nil
}

// parseLines parses the given buffer and returns a slice of lines
func parseLines(buf io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		txt := scanner.Text()

		// check for any line directive and store it for next iteration to
		// re-construct the original file. If it's not a line directive,
		// continue consturcting the original file
		if !strings.HasPrefix(txt, "//line") {
			lines = append(lines, txt)
			continue
		}

		lineNr, err := split(txt)
		if err != nil {
			return nil, err
		}

		for i := len(lines); i < lineNr-1; i++ {
			lines = append(lines, "")
		}

		lines = lines[:lineNr-1]
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("invalid scanner inputl: %s", err)
	}

	return lines, nil
}

// split splits the given line directive and returns the line number
// see https://golang.org/cmd/compile/#hdr-Compiler_Directives for more
// information
// NOTE(arslan): this only splits the line directive that the go.Parser
// outputs. If the go parser changes the format of the line directive, make
// sure to fix it in the below function
func split(line string) (int, error) {
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] != ':' {
			continue
		}

		nr, err := strconv.Atoi(line[i+1:])
		if err != nil {
			return 0, err
		}

		return nr, nil
	}

	return 0, fmt.Errorf("couldn't parse line: '%s'", line)
}

// deref takes an expression, and removes all its leading "*" and "[]"
// operator. Uuse case : if found expression is a "*t" or "[]t", we need to
// check if "t" contains a struct expression.
func deref(x ast.Expr) ast.Expr {
	switch t := x.(type) {
	case *ast.StarExpr:
		return deref(t.X)
	case *ast.ArrayType:
		return deref(t.Elt)
	}
	return x
}
