# reveal [![GitHub Actions](https://github.com/rekki/reveal/actions/workflows/ci.yml/badge.svg)](https://github.com/rekki/reveal/actions/workflows/ci.yml)

[reveal](https://github.com/rekki/reveal) is a statical analysis tool for Go
codebases that aims at extracting valuable information before runtime.

### Features

- Generates [OpenAPI 3.0](https://swagger.io/specification/) schemas from
  [Gin](https://github.com/gin-gonic/gin) source code

## Development

```
go run . ./tests/gin-json | tee /tmp/openapi.json
redoc-cli serve -w /tmp/openapi.json # npm i -g redoc-cli
```

## Implementation Notes

reveal uses
[golang.org/x/tools/go/packages](https://pkg.go.dev/golang.org/x/tools/go/packages)
to statically parse and resolve types for the packages provided to this tool. So
it uses the same base implementation as the Go compiler. Because we are _not_
doing this a runtime, some information might not be available (e.g.:
conditionnal routes based on runtime parameters). Multiple HTTP servers from the
same service will also be merged into a single OpenAPI schema.
