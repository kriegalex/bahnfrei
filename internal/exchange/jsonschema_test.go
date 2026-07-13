// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import "testing"

const testSchema = `{
  "type": "object",
  "required": ["name", "kind"],
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "kind": { "type": "string", "enum": ["a", "b"] },
    "count": { "type": ["integer", "null"] },
    "tags": { "type": "array", "items": { "$ref": "#/$defs/Tag" } }
  },
  "$defs": {
    "Tag": { "type": "object", "required": ["label"], "properties": { "label": { "type": "string" } } }
  }
}`

func TestValidateJSONSchemaAcceptsConformingInstance(t *testing.T) {
	instance := `{"name":"x","kind":"a","count":3,"tags":[{"label":"t1"}]}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err != nil {
		t.Fatalf("ValidateJSONSchema: %v", err)
	}
}

func TestValidateJSONSchemaAcceptsNullForNullableType(t *testing.T) {
	instance := `{"name":"x","kind":"b","count":null}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err != nil {
		t.Fatalf("ValidateJSONSchema: %v", err)
	}
}

func TestValidateJSONSchemaRejectsMissingRequired(t *testing.T) {
	instance := `{"kind":"a"}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err == nil {
		t.Error("want error for missing required property \"name\"")
	}
}

func TestValidateJSONSchemaRejectsWrongType(t *testing.T) {
	instance := `{"name":1,"kind":"a"}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err == nil {
		t.Error("want error for wrong type on \"name\"")
	}
}

func TestValidateJSONSchemaRejectsBadEnum(t *testing.T) {
	instance := `{"name":"x","kind":"z"}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err == nil {
		t.Error("want error for \"kind\" not in enum")
	}
}

func TestValidateJSONSchemaRejectsAdditionalProperty(t *testing.T) {
	instance := `{"name":"x","kind":"a","surprise":true}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err == nil {
		t.Error("want error for an unexpected property under additionalProperties: false")
	}
}

func TestValidateJSONSchemaRejectsBadRefItem(t *testing.T) {
	instance := `{"name":"x","kind":"a","tags":[{"nope":1}]}`
	if err := ValidateJSONSchema([]byte(testSchema), []byte(instance)); err == nil {
		t.Error("want error for a $ref-validated array item missing its own required field")
	}
}

func TestValidateJSONSchemaRejectsMalformedInstanceJSON(t *testing.T) {
	if err := ValidateJSONSchema([]byte(testSchema), []byte(`{not json`)); err == nil {
		t.Error("want error for malformed instance JSON")
	}
}
