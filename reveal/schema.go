package reveal

import (
	"fmt"
	"go/types"

	"github.com/getkin/kin-openapi/openapi3"
)

func schemaFromType(ty types.Type, tag string) *openapi3.SchemaRef {
	switch t := flattenType(ty).(type) {

	case *types.Basic: // https://swagger.io/specification/#data-types
		var value *openapi3.Schema
		switch t.Name() {
		case "bool":
			value = openapi3.NewBoolSchema()
		case "int", "int8", "int16", "intptr", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "rune", "byte":
			value = openapi3.NewIntegerSchema()
		case "int32":
			value = openapi3.NewInt32Schema()
		case "int64":
			value = openapi3.NewInt64Schema()
		case "float32", "float64":
			value = openapi3.NewFloat64Schema()
		case "string":
			value = openapi3.NewStringSchema()
		default:
			panic(fmt.Errorf("unsupported basic type %#v", t))
		}
		return &openapi3.SchemaRef{Value: value}

	case *types.Slice:
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  openapi3.TypeArray,
				Items: schemaFromType(t.Elem(), tag),
			},
		}

	case *types.Struct:
		out := &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:       openapi3.TypeObject,
				Properties: openapi3.Schemas{},
			},
		}
		for i := 0; i < t.NumFields(); i++ {
			field := t.Field(i)
			// ignore private fields
			if !field.Exported() {
				continue
			}
			out.Value.Properties[field.Name()] = schemaFromType(field.Type(), tag)
		}
		return out
	}

	panic(fmt.Errorf("unsupported type %#v", ty))
}

func flattenType(ty types.Type) types.Type {
	for {
		if ptr, ok := ty.(*types.Pointer); ok && ptr != nil {
			ty = ptr.Elem()
		} else if named, ok := ty.(*types.Named); ok && named != nil {
			ty = named.Underlying()
		} else {
			break
		}
	}
	return ty
}
