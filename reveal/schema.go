package reveal

import (
	"fmt"
	"go/types"

	"github.com/fatih/structtag"
	"github.com/getkin/kin-openapi/openapi3"
)

type SchemaRegistry struct {
	Schemas openapi3.Schemas
}

func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		Schemas: openapi3.Schemas{},
	}
}

func (sr *SchemaRegistry) ToSchemaRef(ty types.Type, tag string) *openapi3.SchemaRef {
	ty = flattenPointers(ty)

	if named, ok := ty.(*types.Named); ok && named != nil {
		name := named.Obj().Name()
		if _, ok := sr.Schemas[name]; !ok {
			sr.Schemas[name] = &openapi3.SchemaRef{}
			sr.Schemas[name] = sr.ToSchemaRef(named.Underlying(), tag)
		}
		return &openapi3.SchemaRef{Ref: fmt.Sprintf("#/components/schemas/%s", name)}
	}

	switch t := ty.(type) {

	case *types.Basic: // https://swagger.io/specification/#data-types
		switch t.Name() {
		case "bool":
			return &openapi3.SchemaRef{Value: openapi3.NewBoolSchema()}
		case "int", "int8", "int16", "intptr", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "rune", "byte":
			return &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()}
		case "int32":
			return &openapi3.SchemaRef{Value: openapi3.NewInt32Schema()}
		case "int64":
			return &openapi3.SchemaRef{Value: openapi3.NewInt64Schema()}
		case "float32", "float64":
			return &openapi3.SchemaRef{Value: openapi3.NewFloat64Schema()}
		case "string":
			return &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
		default:
			return nil
		}

	case *types.Interface:
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:        openapi3.TypeObject,
				Description: fmt.Sprintf("%v", ty),
			},
		}

	case *types.Map:
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:                 openapi3.TypeObject,
				AdditionalProperties: sr.ToSchemaRef(t.Elem(), tag),
			},
		}

	case *types.Slice:
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  openapi3.TypeArray,
				Items: sr.ToSchemaRef(t.Elem(), tag),
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

			tags, err := structtag.Parse(t.Tag(i))
			if err != nil {
				continue
			}

			property := field.Name()
			for _, key := range tags.Keys() {
				if key == tag {
					if value, err := tags.Get(tag); err == nil {
						property = value.Name
						break
					}
				}
			}

			if property != "-" {
				out.Value.Properties[property] = sr.ToSchemaRef(field.Type(), tag)
			}
		}

		return out
	}

	panic(fmt.Errorf("unsupported type %#v", ty))
}

func flattenPointers(ty types.Type) types.Type {
	for {
		if ptr, ok := ty.(*types.Pointer); ok && ptr != nil {
			ty = ptr.Elem()
		} else {
			break
		}
	}
	return ty
}
