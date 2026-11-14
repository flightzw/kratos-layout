package docs

import _ "embed"

//go:embed openapi/openapi.json
var APISpec []byte
