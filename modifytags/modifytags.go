package modifytags

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/fatih/camelcase"
	"github.com/fatih/structtag"
)

// A Transform determines how Go field names will be translated into
// names used in struct tags. For example, the [SnakeCase] transform
// converts the field MyField into the json tag json:"my_field".
type Transform int

const (
	SnakeCase  = iota // MyField -> my_field
	CamelCase         // MyField -> myField
	LispCase          // MyField -> my-field
	PascalCase        // MyField -> MyField
	TitleCase         // MyField -> My Field
	Keep              // keep the existing field name
)

// A Modification defines how struct tags should be modified for a given input struct.
type Modification struct {
	Add        []string            // tags to add
	AddOptions map[string][]string // options to add, per tag

	Remove        []string            // tags to remove
	RemoveOptions map[string][]string // options to remove, per tag

	Overwrite            bool // if set, replace existing tags when adding
	SkipUnexportedFields bool // if set, do not modify tags on unexported struct fields

	Transform    Transform // transform rule for adding tags
	Sort         bool      // if set, sort tags in ascending order by key name
	ValueFormat  string    // format for the tag's value, after transformation; for example "column:{field}"
	Clear        bool      // if set, clear all tags. tags are cleared before any new tags are added
	ClearOptions bool      // if set, clear all tag options; options are cleared before any new options are added
}

// Apply applies the struct tag modifications of the receiver to all
// struct fields contained within the given node between start and end position, modifying its input.
func (mod *Modification) Apply(fset *token.FileSet, node ast.Node, start, end token.Pos) error {
	err := mod.validate()
	if err != nil {
		return err
	}

	return mod.rewrite(fset, node, start, end)
}

// rewrite rewrites the node for structs between the start and end positions
func (mod *Modification) rewrite(fset *token.FileSet, node ast.Node, start, end token.Pos) error {
	var errs []error

	rewriteFunc := func(n ast.Node) bool {
		x, ok := n.(*ast.StructType)
		if !ok {
			return true
		}

		for _, f := range x.Fields.List {
			if !(start <= f.End() && f.Pos() <= end) {
				continue // not in range
			}

			fieldName := ""
			if len(f.Names) != 0 {
				for _, field := range f.Names {
					if !mod.SkipUnexportedFields || isPublicName(field.Name) {
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

				if !mod.SkipUnexportedFields {
					fieldName = ident.Name
				}
			}

			// nothing to process, continue with next field
			if fieldName == "" {
				continue
			}

			curTag := ""
			if f.Tag != nil {
				curTag = f.Tag.Value
			}

			res, err := mod.processField(fieldName, curTag)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s:%d:%d:%s",
					fset.Position(f.Pos()).Filename,
					fset.Position(f.Pos()).Line,
					fset.Position(f.Pos()).Column,
					err))
				continue
			}

			if res == "" {
				f.Tag = nil
			} else {
				if f.Tag == nil {
					f.Tag = &ast.BasicLit{}
				}
				f.Tag.Value = res
			}
		}

		return true
	}

	ast.Inspect(node, rewriteFunc)

	if len(errs) > 0 {
		return &RewriteErrors{Errs: errs}
	}
	return nil
}

// processField returns the new struct tag value for the given field
func (mod *Modification) processField(fieldName, tagVal string) (string, error) {
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

	tags = mod.removeTags(tags)
	tags, err = mod.removeTagOptions(tags)
	if err != nil {
		return "", err
	}

	tags = mod.clearTags(tags)
	tags = mod.clearOptions(tags)

	tags, err = mod.addTags(fieldName, tags)
	if err != nil {
		return "", err
	}

	tags, err = mod.addTagOptions(tags)
	if err != nil {
		return "", err
	}

	if mod.Sort {
		sort.Sort(tags)
	}

	res := tags.String()
	if res != "" {
		res = quote(tags.String())
	}

	return res, nil
}

func (mod *Modification) removeTags(tags *structtag.Tags) *structtag.Tags {
	if len(mod.Remove) == 0 {
		return tags
	}

	tags.Delete(mod.Remove...)
	return tags
}

func (mod *Modification) clearTags(tags *structtag.Tags) *structtag.Tags {
	if !mod.Clear {
		return tags
	}

	tags.Delete(tags.Keys()...)
	return tags
}

func (mod *Modification) clearOptions(tags *structtag.Tags) *structtag.Tags {
	if !mod.ClearOptions {
		return tags
	}

	for _, t := range tags.Tags() {
		t.Options = nil
	}

	return tags
}

func (mod *Modification) removeTagOptions(tags *structtag.Tags) (*structtag.Tags, error) {
	if len(mod.RemoveOptions) == 0 {
		return tags, nil
	}

	for key, val := range mod.RemoveOptions {
		for _, option := range val {
			tags.DeleteOptions(key, option)
		}
	}

	return tags, nil
}

func (mod *Modification) addTagOptions(tags *structtag.Tags) (*structtag.Tags, error) {
	if len(mod.AddOptions) == 0 {
		return tags, nil
	}

	for key, val := range mod.AddOptions {
		tags.AddOptions(key, val...)
	}

	return tags, nil
}

func (mod *Modification) addTags(fieldName string, tags *structtag.Tags) (*structtag.Tags, error) {
	if len(mod.Add) == 0 {
		return tags, nil
	}

	split := camelcase.Split(fieldName)
	name := ""

	switch mod.Transform {

	case LispCase:
		var lowerSplit []string
		for _, s := range split {
			lowerSplit = append(lowerSplit, strings.ToLower(s))
		}

		name = strings.Join(lowerSplit, "-")
	case CamelCase:
		var titled []string
		for _, s := range split {
			titled = append(titled, strings.Title(s))
		}

		titled[0] = strings.ToLower(titled[0])

		name = strings.Join(titled, "")
	case PascalCase:
		var titled []string
		for _, s := range split {
			titled = append(titled, strings.Title(s))
		}

		name = strings.Join(titled, "")
	case TitleCase:
		var titled []string
		for _, s := range split {
			titled = append(titled, strings.Title(s))
		}

		name = strings.Join(titled, " ")
	case Keep:
		name = fieldName
	case SnakeCase:
		fallthrough
	default:
		// Use snakecase as the default.
		var lowerSplit []string
		for _, s := range split {
			s = strings.Trim(s, "_")
			if s == "" {
				continue
			}
			lowerSplit = append(lowerSplit, strings.ToLower(s))
		}

		name = strings.Join(lowerSplit, "_")
	}

	if mod.ValueFormat != "" {
		prevName := name
		name = strings.ReplaceAll(mod.ValueFormat, "{field}", name)
		if name == mod.ValueFormat {
			// support old style for backward compatibility
			name = strings.ReplaceAll(mod.ValueFormat, "$field", prevName)
		}
	}

	for _, key := range mod.Add {
		split = strings.SplitN(key, ":", 2)
		if len(split) >= 2 {
			key = split[0]
			name = strings.Join(split[1:], "")
		}

		tag, err := tags.Get(key)
		if err != nil {
			// tag doesn't exist, create a new one
			tag = &structtag.Tag{
				Key:  key,
				Name: name,
			}
		} else if mod.Overwrite {
			tag.Name = name
		}

		if err := tags.Set(tag); err != nil {
			return nil, err
		}
	}

	return tags, nil
}

func isPublicName(name string) bool {
	for _, c := range name {
		return unicode.IsUpper(c)
	}
	return false
}

// validate determines whether the Modification is valid or not.
func (mod *Modification) validate() error {
	if len(mod.Add) == 0 &&
		len(mod.AddOptions) == 0 &&
		!mod.Clear &&
		!mod.ClearOptions &&
		len(mod.RemoveOptions) == 0 &&
		len(mod.Remove) == 0 {
		return errors.New("one of " +
			"[-add-tags, -add-options, -remove-tags, -remove-options, -clear-tags, -clear-options]" +
			" should be defined")
	}
	return nil
}

func quote(tag string) string {
	return "`" + tag + "`"
}

// RewriteErrors are errors that occurred while rewriting struct field tags.
type RewriteErrors struct {
	Errs []error
}

func (r *RewriteErrors) Error() string {
	var buf bytes.Buffer
	for _, e := range r.Errs {
		buf.WriteString(fmt.Sprintf("%s\n", e.Error()))
	}
	return buf.String()
}
