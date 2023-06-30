package main

import (
	"flag"
	"fmt"
	"github.com/rs/zerolog/log"
	maputils "gitlab.com/kordax/basic-utils/map-utils"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var filePath = flag.String("f", "", "file to read")
var structName = flag.String("s", "", "struct name to parse")
var outputFile = flag.String("o", "", "output file (optional)")
var threshold = flag.Int("g", math.MaxInt, "specifies groups threshold to parse PascalCase names into specific groups.\n"+
	"if positive value is provided then fields will be grouped into separate messages in case `g` or more groups were found.")

func main() {
	flag.Parse()
	if len(os.Args) < 3 {
		log.Fatal().Msg("Not enough arguments provided. Usage: go run main.go <input-file.go> <output-file.proto>")
		return
	}

	protoDef, err := generateProtobufDefinition(*filePath, *structName, 0, *threshold)
	if err != nil {
		log.Fatal().Msgf("Error: %s", err)
		return
	}

	protoDef = "syntax = 'proto3';\n\npackage my_package;\n\n" + protoDef

	fmt.Println(protoDef)
	if *outputFile != "" {
		saveerr := saveToFile(*outputFile, protoDef)
		if saveerr != nil {
			log.Fatal().Msgf("Failed to save protobuf to file: %s", saveerr)
		}
		log.Info().Msgf("Protobuf definition generated and saved to %s", *outputFile)
	}
}

func checkRecursiveStruct(structType *ast.StructType, targetName string) (bool, string) {
	return checkRecursiveStructRecursive(structType, structType, "", targetName)
}

func checkRecursiveStructRecursive(structType *ast.StructType, targetType ast.Node, parentFieldName string, targetName string) (bool, string) {
	for _, field := range structType.Fields.List {
		if fieldType, ok := field.Type.(*ast.StarExpr); ok && fieldType.X != nil && fieldType.X.(*ast.Ident).Name == targetName {
			// Found a nested struct with the same type as the target struct
			fieldName := field.Names[0].Name
			return true, fieldName
		}
	}

	return false, ""
}

func generateProtobufDefinition(filePath string, structName string, depth int, threshold int) (string, error) {
	if threshold == 0 {
		return "", fmt.Errorf("grouping value cannot be 0")
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}

	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == structName {
					if structType, ok := typeSpec.Type.(*ast.StructType); ok {
						if rec, name := checkRecursiveStruct(structType, structName); rec {
							panic(fmt.Errorf("struct contains a nested field of a same type that provokes infinite recusion: %s", name))
						}
						return generateProtobufStruct(structName, structType, depth, threshold), nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("struct '%s' not found in the Go source file", structName)
}

func generateProtobufStruct(structName string, structType *ast.StructType, depth int, threshold int) string {
	var lines []string
	fieldNumber := 1

	SortFieldsByName(structType.Fields.List)
	groups := GroupFieldsByPascalCase(structType.Fields.List, threshold)
	// Create a new map with sorted keys
	sortedUngrouped := groups["ungrouped"]
	sort.Slice(sortedUngrouped, func(i, j int) bool {
		return sortedUngrouped[i].Names[0].Name < sortedUngrouped[j].Names[0].Name
	})
	for _, field := range sortedUngrouped {
		fieldNames := getFieldNames(field.Names)
		fieldType := fieldTypeToString(field.Type, depth, threshold)

		line := ""
		pbType := convertTypeToProtobuf(fieldType)
		switch t := field.Type.(type) {
		case *ast.StarExpr:
			ident := t.X.(*ast.Ident)
			if t.X.(*ast.Ident).Obj == nil {
				line = fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", pbType, convertToSnakeCase(strings.Join(fieldNames, ", ")), fieldNumber)
				break
			}
			line = fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s\n", pbType)
			line += fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", ident.Name, convertToSnakeCase(strings.Join(fieldNames, ", ")), fieldNumber)

		case *ast.Ident:
			if t.Obj == nil {
				line = fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", pbType, convertToSnakeCase(strings.Join(fieldNames, ", ")), fieldNumber)
				break
			}
			line = fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s\n", pbType)
			line += fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", t.Name, convertToSnakeCase(strings.Join(fieldNames, ", ")), fieldNumber)
		default:
			line = fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", pbType, convertToSnakeCase(strings.Join(fieldNames, ", ")), fieldNumber)
		}
		lines = append(lines, line)
		fieldNumber++
	}

	// Sort the keys alphabetically
	sortedGroups := maputils.Keys(groups)
	sort.Strings(sortedGroups)
	for _, group := range sortedGroups {
		if group == "ungrouped" {
			continue
		}

		nestedFieldNumber := 1
		var nestedLines []string
		for i, field := range groups[group] {
			fieldNames := getFieldNames(field.Names)
			fieldType := fieldTypeToString(field.Type, depth, threshold)
			line := fmt.Sprintf(strings.Repeat(" ", (depth+2)*2)+"%s %s = %d;", convertTypeToProtobuf(fieldType), convertToSnakeCase(strings.Join(fieldNames, ", ")), i+1)
			nestedLines = append(nestedLines, line)
			nestedFieldNumber++
		}
		lines = append(lines, fmt.Sprintf(strings.Repeat(" ", depth+1*2)+"message %s {\n%s\n"+strings.Repeat(" ", depth+1*2)+"}", group, strings.Join(nestedLines, "\n")))
		lines = append(lines, fmt.Sprintf(strings.Repeat(" ", (depth+1)*2)+"%s %s = %d;", group, convertToSnakeCase(group), fieldNumber))
		fieldNumber++
	}

	protoDef := fmt.Sprintf("message %s {\n%s\n"+strings.Repeat(" ", depth*2)+"}", structName, strings.Join(lines, "\n"))
	depth++
	return protoDef
}

func getFieldNames(names []*ast.Ident) []string {
	var fieldNames []string
	for _, name := range names {
		fieldNames = append(fieldNames, name.Name)
	}
	return fieldNames
}

func fieldTypeToString(fieldType ast.Expr, depth, threshold int) string {
	switch t := fieldType.(type) {
	case *ast.Ident:
		if t.Obj != nil {
			if t.Obj.Kind == ast.Typ {
				if structType, ok := t.Obj.Decl.(*ast.TypeSpec).Type.(*ast.StructType); ok {
					// Pass the struct type to the method
					return generateProtobufStruct(t.Name, structType, depth+1, threshold)
				}
			}
		}
		return t.Name
	case *ast.StarExpr:
		return fieldTypeToString(t.X, 0, threshold)
	case *ast.ArrayType:
		return "repeated " + fieldTypeToString(t.Elt, 0, threshold)
	case *ast.MapType:
		keyType := fieldTypeToString(t.Key, 0, threshold)
		valueType := fieldTypeToString(t.Value, 0, threshold)
		return fmt.Sprintf("map[%s]%s", keyType, valueType)
	case *ast.IndexExpr:
		if ident, ok := t.X.(*ast.SelectorExpr); ok {
			if xIdent, ok := ident.X.(*ast.Ident); ok && xIdent.Name == "opt" {
				if ident.Sel.Name == "Opt" {
					if genericType := fieldTypeToString(t.Index, 0, threshold); genericType != "" {
						return convertTypeToProtobuf(genericType)
					}
				}
			}
		}
		return fieldTypeToString(t.X, 0, threshold)
	case *ast.SelectorExpr:
		if xIdent, ok := t.X.(*ast.Ident); ok && xIdent.Name == "opt" {
			ident := t.Sel
			if ident.Name == "Opt" {
				return "opt.Opt"
			}
			return ident.Name
		}
		if identType := fieldTypeToString(t.X, 0, threshold); identType != "" {
			return fmt.Sprintf("%s.%s", identType, t.Sel.Name)
		}
		return ""
	default:
		return ""
	}
}

func convertTypeToProtobuf(goType string) string {
	switch goType {
	case "string":
		return "string"
	case "int", "int8", "int16", "int32":
		return "int32"
	case "uint", "uint8", "uint16", "uint32":
		return "uint32"
	case "int64":
		return "int64"
	case "uint64":
		return "uint64"
	case "float32", "float64":
		return "double"
	case "bool":
		return "bool"
	case "time.Time":
		return "int64"
	default:
		return goType
	}
}

func saveToFile(filename string, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}

func convertToSnakeCase(text string) string {
	var result strings.Builder

	for i, char := range text {
		if unicode.IsUpper(char) {
			if i > 0 && unicode.IsLower(rune(text[i-1])) {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(char))
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// SortFieldsByName sorts a slice of ast.Field by the field name
func SortFieldsByName(fields []*ast.Field) {
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Names[0].Name < fields[j].Names[0].Name
	})
}

// GroupFieldsByPascalCase parses the field names in PascalCase and groups them accordingly
func GroupFieldsByPascalCase(fields []*ast.Field, threshold int) map[string][]*ast.Field {
	groups := make(map[string][]*ast.Field)
	ungrouped := make([]*ast.Field, 0)

	// Regular expression to match PascalCase names
	pattern := regexp.MustCompile(`([A-Z][a-z]*)`)

	for _, field := range fields {
		// Extract the field name
		name := field.Names[0].Name

		// Find all matches of PascalCase words in the field name
		matches := pattern.FindAllString(name, -1)

		if len(matches) >= threshold {
			// Group the matches together into a single string
			group := strings.Join(matches[:threshold], "")

			// Append the field to the group
			groups[group] = append(groups[group], field)
		} else {
			// Add the field to the ungrouped group
			ungrouped = append(ungrouped, field)
		}
	}

	// Move single-element groups to the ungrouped slice
	for group, fieldSlice := range groups {
		if len(fieldSlice) == 1 {
			ungrouped = append(ungrouped, fieldSlice[0])
			delete(groups, group)
		}
	}
	groups["ungrouped"] = ungrouped
	return groups
}
