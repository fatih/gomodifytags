package gomodifytags

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/fatih/camelcase"
	"github.com/fatih/structtag"
	"golang.org/x/tools/go/buildutil"
)

// structType contains a structType node and it's name. It's a convenient
// helper type, because *ast.StructType doesn't contain the name of the struct
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

// Config configures gomodify tags and describes how tags should be modified.
type Config struct {
	fset *token.FileSet

	File     string
	Output   string
	Quiet    bool
	Write    bool
	Modified io.Reader

	Offset     int
	StructName string
	FieldName  string
	Line       string
	Start, End int
	All        bool

	Remove        []string
	RemoveOptions []string

	Add                  []string
	AddOptions           []string
	Override             bool
	SkipUnexportedFields bool

	Transform   string
	Sort        bool
	ValueFormat string
	Clear       bool
	ClearOption bool
}

// Run runs gomodofiytags with the config.
func (c *Config) Run() error {
	err := c.validate()
	if err != nil {
		return err
	}

	node, err := c.parse()
	if err != nil {
		return err
	}

	start, end, err := c.findSelection(node)
	if err != nil {
		return err
	}

	rewrittenNode, errs := c.rewrite(node, start, end)
	if errs != nil {
		if _, ok := errs.(*rewriteErrors); !ok {
			return errs
		}
	}

	out, err := c.format(rewrittenNode, errs)
	if err != nil {
		return err
	}

	if !c.Quiet {
		fmt.Println(out)
	}

	return nil
}

func (c *Config) parse() (ast.Node, error) {
	c.fset = token.NewFileSet()
	var contents interface{}
	if c.Modified != nil {
		archive, err := buildutil.ParseOverlayArchive(c.Modified)
		if err != nil {
			return nil, fmt.Errorf("failed to parse -modified archive: %v", err)
		}
		fc, ok := archive[c.File]
		if !ok {
			return nil, fmt.Errorf("couldn't find %s in archive", c.File)
		}
		contents = fc
	}

	return parser.ParseFile(c.fset, c.File, contents, parser.ParseComments)
}

// findSelection returns the start and end position of the fields that are
// suspect to change. It depends on the line, struct or offset selection.
func (c *Config) findSelection(node ast.Node) (int, int, error) {
	if c.Line != "" {
		return c.lineSelection(node)
	} else if c.Offset != 0 {
		return c.offsetSelection(node)
	} else if c.StructName != "" {
		return c.structSelection(node)
	} else if c.All {
		return c.allSelection(node)
	} else {
		return 0, 0, errors.New("-line, -offset, -struct or -all is not passed")
	}
}

func (c *Config) process(fieldName, tagVal string) (string, error) {
	var tag string
	if tagVal != "" {
		var err error
		tag, err = strconv.Unquote(tagVal)
		if err != nil {
			return "", err
		}
	}

	tags, err := structtag.Parse(tag)
	if err != nil {
		return "", err
	}

	tags = c.removeTags(tags)
	tags, err = c.removeTagOptions(tags)
	if err != nil {
		return "", err
	}

	tags = c.clearTags(tags)
	tags = c.clearOptions(tags)

	tags, err = c.addTags(fieldName, tags)
	if err != nil {
		return "", err
	}

	tags, err = c.addTagOptions(tags)
	if err != nil {
		return "", err
	}

	if c.Sort {
		sort.Sort(tags)
	}

	res := tags.String()
	if res != "" {
		res = quote(tags.String())
	}

	return res, nil
}

func (c *Config) removeTags(tags *structtag.Tags) *structtag.Tags {
	if c.Remove == nil || len(c.Remove) == 0 {
		return tags
	}

	tags.Delete(c.Remove...)
	return tags
}

func (c *Config) clearTags(tags *structtag.Tags) *structtag.Tags {
	if !c.Clear {
		return tags
	}

	tags.Delete(tags.Keys()...)
	return tags
}

func (c *Config) clearOptions(tags *structtag.Tags) *structtag.Tags {
	if !c.ClearOption {
		return tags
	}

	for _, t := range tags.Tags() {
		t.Options = nil
	}

	return tags
}

func (c *Config) removeTagOptions(tags *structtag.Tags) (*structtag.Tags, error) {
	if c.RemoveOptions == nil || len(c.RemoveOptions) == 0 {
		return tags, nil
	}

	for _, val := range c.RemoveOptions {
		// syntax key=option
		splitted := strings.Split(val, "=")
		if len(splitted) < 2 {
			return nil, errors.New("wrong syntax to remove an option. i.e key=option")
		}

		key := splitted[0]
		option := strings.Join(splitted[1:], "=")

		tags.DeleteOptions(key, option)
	}

	return tags, nil
}

func (c *Config) addTagOptions(tags *structtag.Tags) (*structtag.Tags, error) {
	if c.AddOptions == nil || len(c.AddOptions) == 0 {
		return tags, nil
	}

	for _, val := range c.AddOptions {
		// syntax key=option
		splitted := strings.Split(val, "=")
		if len(splitted) < 2 {
			return nil, errors.New("wrong syntax to add an option. i.e key=option")
		}

		key := splitted[0]
		option := strings.Join(splitted[1:], "=")

		tags.AddOptions(key, option)
	}

	return tags, nil
}

func (c *Config) addTags(fieldName string, tags *structtag.Tags) (*structtag.Tags, error) {
	if c.Add == nil || len(c.Add) == 0 {
		return tags, nil
	}

	splitted := camelcase.Split(fieldName)
	name := ""

	unknown := false
	switch c.Transform {
	case "snakecase":
		var lowerSplitted []string
		for _, s := range splitted {
			lowerSplitted = append(lowerSplitted, strings.ToLower(s))
		}

		name = strings.Join(lowerSplitted, "_")
	case "lispcase":
		var lowerSplitted []string
		for _, s := range splitted {
			lowerSplitted = append(lowerSplitted, strings.ToLower(s))
		}

		name = strings.Join(lowerSplitted, "-")
	case "camelcase":
		var titled []string
		for _, s := range splitted {
			titled = append(titled, strings.Title(s))
		}

		titled[0] = strings.ToLower(titled[0])

		name = strings.Join(titled, "")
	case "pascalcase":
		var titled []string
		for _, s := range splitted {
			titled = append(titled, strings.Title(s))
		}

		name = strings.Join(titled, "")
	case "titlecase":
		var titled []string
		for _, s := range splitted {
			titled = append(titled, strings.Title(s))
		}

		name = strings.Join(titled, " ")
	case "keep":
		name = fieldName
	default:
		unknown = true
	}

	if c.ValueFormat != "" {
		prevName := name
		name = strings.ReplaceAll(c.ValueFormat, "{field}", name)
		if name == c.ValueFormat {
			// support old style for backward compatibility
			name = strings.ReplaceAll(c.ValueFormat, "$field", prevName)
		}
	}

	for _, key := range c.Add {
		splitted = strings.SplitN(key, ":", 2)
		if len(splitted) >= 2 {
			key = splitted[0]
			name = strings.Join(splitted[1:], "")
		} else if unknown {
			// the user didn't pass any value but want to use an unknown
			// transform. We don't return above in the default as the user
			// might pass a value
			return nil, fmt.Errorf("unknown transform option %q", c.Transform)
		}

		tag, err := tags.Get(key)
		if err != nil {
			// tag doesn't exist, create a new one
			tag = &structtag.Tag{
				Key:  key,
				Name: name,
			}
		} else if c.Override {
			tag.Name = name
		}

		if err := tags.Set(tag); err != nil {
			return nil, err
		}
	}

	return tags, nil
}

// collectStructs collects and maps structType nodes to their positions
func collectStructs(node ast.Node) map[token.Pos]*structType {
	structs := make(map[token.Pos]*structType)

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

func (c *Config) format(file ast.Node, rwErrs error) (string, error) {
	switch c.Output {
	case "source":
		var buf bytes.Buffer
		err := format.Node(&buf, c.fset, file)
		if err != nil {
			return "", err
		}

		if c.Write {
			err = ioutil.WriteFile(c.File, buf.Bytes(), 0)
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
		cfg := printer.Config{Mode: printer.SourcePos | printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
		err := cfg.Fprint(&buf, c.fset, file)
		if err != nil {
			return "", err
		}

		lines, err := parseLines(&buf)
		if err != nil {
			return "", err
		}

		// prevent selection to be larger than the actual number of lines
		if c.Start > len(lines) || c.End > len(lines) {
			return "", errors.New("line selection is invalid")
		}

		out := &output{
			Start: c.Start,
			End:   c.End,
			Lines: lines[c.Start-1 : c.End],
		}

		if rwErrs != nil {
			if r, ok := rwErrs.(*rewriteErrors); ok {
				for _, err := range r.errs {
					out.Errors = append(out.Errors, err.Error())
				}
			}
		}

		o, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return "", err
		}

		return string(o), nil
	default:
		return "", fmt.Errorf("unknown output mode: %s", c.Output)
	}
}

func (c *Config) lineSelection(file ast.Node) (int, int, error) {
	var err error
	splitted := strings.Split(c.Line, ",")

	start, err := strconv.Atoi(splitted[0])
	if err != nil {
		return 0, 0, err
	}

	end := start
	if len(splitted) == 2 {
		end, err = strconv.Atoi(splitted[1])
		if err != nil {
			return 0, 0, err
		}
	}

	if start > end {
		return 0, 0, errors.New("wrong range. start line cannot be larger than end line")
	}

	return start, end, nil
}

func (c *Config) structSelection(file ast.Node) (int, int, error) {
	structs := collectStructs(file)

	var encStruct *ast.StructType
	for _, st := range structs {
		if st.name == c.StructName {
			encStruct = st.node
		}
	}

	if encStruct == nil {
		return 0, 0, errors.New("struct name does not exist")
	}

	// if field name has been specified as well, only select the given field
	if c.FieldName != "" {
		return c.fieldSelection(encStruct)
	}

	start := c.fset.Position(encStruct.Pos()).Line
	end := c.fset.Position(encStruct.End()).Line

	return start, end, nil
}

func (c *Config) fieldSelection(st *ast.StructType) (int, int, error) {
	var encField *ast.Field
	for _, f := range st.Fields.List {
		for _, field := range f.Names {
			if field.Name == c.FieldName {
				encField = f
			}
		}
	}

	if encField == nil {
		return 0, 0, fmt.Errorf("struct %q doesn't have field name %q", c.StructName, c.FieldName)
	}

	start := c.fset.Position(encField.Pos()).Line
	end := c.fset.Position(encField.End()).Line

	return start, end, nil
}

func (c *Config) offsetSelection(file ast.Node) (int, int, error) {
	structs := collectStructs(file)

	var encStruct *ast.StructType
	for _, st := range structs {
		structBegin := c.fset.Position(st.node.Pos()).Offset
		structEnd := c.fset.Position(st.node.End()).Offset

		if structBegin <= c.Offset && c.Offset <= structEnd {
			encStruct = st.node
			break
		}
	}

	if encStruct == nil {
		return 0, 0, errors.New("offset is not inside a struct")
	}

	// offset selects all fields
	start := c.fset.Position(encStruct.Pos()).Line
	end := c.fset.Position(encStruct.End()).Line

	return start, end, nil
}

// allSelection selects all structs inside a file
func (c *Config) allSelection(file ast.Node) (int, int, error) {
	start := 1
	end := c.fset.File(file.Pos()).LineCount()

	return start, end, nil
}

func isPublicName(name string) bool {
	for _, c := range name {
		return unicode.IsUpper(c)
	}
	return false
}

// rewrite rewrites the node for structs between the start and end
// positions
func (c *Config) rewrite(node ast.Node, start, end int) (ast.Node, error) {
	errs := &rewriteErrors{errs: make([]error, 0)}

	rewriteFunc := func(n ast.Node) bool {
		x, ok := n.(*ast.StructType)
		if !ok {
			return true
		}

		for _, f := range x.Fields.List {
			line := c.fset.Position(f.Pos()).Line

			if !(start <= line && line <= end) {
				continue
			}

			fieldName := ""
			if len(f.Names) != 0 {
				for _, field := range f.Names {
					if !c.SkipUnexportedFields || isPublicName(field.Name) {
						fieldName = field.Name
						break
					}
				}
			}

			// anonymous field
			if f.Names == nil {
				ident, ok := f.Type.(*ast.Ident)
				if !ok {
					continue
				}

				if !c.SkipUnexportedFields {
					fieldName = ident.Name
				}
			}

			// nothing to process, continue with next line
			if fieldName == "" {
				continue
			}

			if f.Tag == nil {
				f.Tag = &ast.BasicLit{}
			}

			res, err := c.process(fieldName, f.Tag.Value)
			if err != nil {
				errs.Append(fmt.Errorf("%s:%d:%d:%s",
					c.fset.Position(f.Pos()).Filename,
					c.fset.Position(f.Pos()).Line,
					c.fset.Position(f.Pos()).Column,
					err))
				continue
			}

			f.Tag.Value = res
		}

		return true
	}

	ast.Inspect(node, rewriteFunc)

	c.Start = start
	c.End = end

	if len(errs.errs) == 0 {
		return node, nil
	}

	return node, errs
}

// validate validates whether the config is valid or not
func (c *Config) validate() error {
	if c.File == "" {
		return errors.New("no file is passed")
	}

	if c.Line == "" && c.Offset == 0 && c.StructName == "" && !c.All {
		return errors.New("-line, -offset, -struct or -all is not passed")
	}

	if c.Line != "" && c.Offset != 0 ||
		c.Line != "" && c.StructName != "" ||
		c.Offset != 0 && c.StructName != "" {
		return errors.New("-line, -offset or -struct cannot be used together. pick one")
	}

	if (c.Add == nil || len(c.Add) == 0) &&
		(c.AddOptions == nil || len(c.AddOptions) == 0) &&
		!c.Clear &&
		!c.ClearOption &&
		(c.RemoveOptions == nil || len(c.RemoveOptions) == 0) &&
		(c.Remove == nil || len(c.Remove) == 0) {
		return errors.New("one of " +
			"[-add-tags, -add-options, -remove-tags, -remove-options, -clear-tags, -clear-options]" +
			" should be defined")
	}

	if c.FieldName != "" && c.StructName == "" {
		return errors.New("-field is requiring -struct")
	}

	return nil
}

func quote(tag string) string {
	return "`" + tag + "`"
}

type rewriteErrors struct {
	errs []error
}

func (r *rewriteErrors) Error() string {
	var buf bytes.Buffer
	for _, e := range r.errs {
		buf.WriteString(fmt.Sprintf("%s\n", e.Error()))
	}
	return buf.String()
}

func (r *rewriteErrors) Append(err error) {
	if err == nil {
		return
	}

	r.errs = append(r.errs, err)
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
