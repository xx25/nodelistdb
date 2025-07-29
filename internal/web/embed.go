package web

import "embed"

//go:embed templates/*.html
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

// OpenAPISpec will be set by the API package
var OpenAPISpec []byte
