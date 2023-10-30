//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen@v1.15.0 --config=oapi-codegen-types.yaml openapi.yaml
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen@v1.15.0 --config=oapi-codegen-client.yaml openapi.yaml
package api
