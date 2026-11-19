package api

import _ "embed"

// openAPISpec is the OpenAPI document served at /openapi.json. It describes the routes the
// router registers; a test keeps the two in step.
//
//go:embed openapi.json
var openAPISpec []byte
