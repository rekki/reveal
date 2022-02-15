package reveal

import (
	"go/types"

	"github.com/getkin/kin-openapi/openapi3"
)

func schemaFromType(ty types.Type, tag string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{}
}
