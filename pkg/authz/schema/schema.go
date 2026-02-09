package schema

import _ "embed"

// CedarSchema is the Cedar schema for ROSA authorization
// This schema defines the entity types and actions for the ROSA authorization model
//
//go:embed rosa.cedarschema
var CedarSchema string

// CedarSchemaJSON is the JSON representation of the Cedar schema for AVP
//
//go:embed rosa.cedarschema.json
var CedarSchemaJSON string
