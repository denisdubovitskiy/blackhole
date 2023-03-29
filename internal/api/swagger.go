package api

import (
	_ "embed"
)

//go:embed blackhole.swagger.json
var SwaggerUI []byte
