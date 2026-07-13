// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// jsonSchema is a safe, minimal subset of JSON Schema (draft 2020-12):
// type, properties, required, items, enum, $defs/$ref (local refs only)
// and additionalProperties. This is deliberately not a general-purpose
// engine — SYS-144 only needs "the exported omx/v1 document conforms to
// the document this system itself publishes" verified in CI without
// adding a third-party schema-validator dependency (none of this
// project's other exchange formats — FinishLynx, generic CSV — use one
// either; they compare fixtures field-for-field instead). Unsupported
// keywords (oneOf/anyOf/allOf/pattern/format/$id) are simply not
// interpreted; the omx/v1 schema (schema/omx-v1.schema.json) is written
// to stay inside this subset.
type jsonSchema struct {
	Type                 any                    `json:"type,omitempty"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Items                *jsonSchema            `json:"items,omitempty"`
	Enum                 []any                  `json:"enum,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	Defs                 map[string]*jsonSchema `json:"$defs,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
}

// ValidateJSONSchema checks that instance (raw JSON bytes) conforms to
// schema (a raw JSON Schema document, in the subset jsonSchema supports).
// It returns the first violation found, with a JSON-pointer-ish path so a
// failure is locatable.
func ValidateJSONSchema(schema, instance []byte) error {
	var root jsonSchema
	if err := json.Unmarshal(schema, &root); err != nil {
		return fmt.Errorf("jsonschema: parse schema: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(instance))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("jsonschema: parse instance: %w", err)
	}
	return validateNode(&root, &root, value, "$")
}

func validateNode(root, s *jsonSchema, v any, path string) error {
	if s.Ref != "" {
		def, ok := resolveRef(root, s.Ref)
		if !ok {
			return fmt.Errorf("%s: unresolved $ref %q", path, s.Ref)
		}
		return validateNode(root, def, v, path)
	}
	if len(s.Enum) > 0 && !enumContains(s.Enum, v) {
		return fmt.Errorf("%s: value %v is not one of the enumerated values", path, v)
	}
	if s.Type != nil {
		if err := checkType(s.Type, v, path); err != nil {
			return err
		}
	}
	switch vv := v.(type) {
	case map[string]any:
		for _, req := range s.Required {
			if _, ok := vv[req]; !ok {
				return fmt.Errorf("%s: missing required property %q", path, req)
			}
		}
		if s.AdditionalProperties != nil && !*s.AdditionalProperties {
			for k := range vv {
				if _, ok := s.Properties[k]; !ok {
					return fmt.Errorf("%s: unexpected property %q (additionalProperties: false)", path, k)
				}
			}
		}
		for k, propSchema := range s.Properties {
			if val, ok := vv[k]; ok {
				if err := validateNode(root, propSchema, val, path+"."+k); err != nil {
					return err
				}
			}
		}
	case []any:
		if s.Items != nil {
			for i, item := range vv {
				if err := validateNode(root, s.Items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// resolveRef resolves a local "#/$defs/Name" reference against root.
func resolveRef(root *jsonSchema, ref string) (*jsonSchema, bool) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, false
	}
	name := strings.TrimPrefix(ref, prefix)
	def, ok := root.Defs[name]
	return def, ok
}

// checkType validates v against a JSON Schema "type" keyword, which is
// either a single type name or an array of type names (the standard way
// to express nullability, e.g. ["string","null"]).
func checkType(t any, v any, path string) error {
	var names []string
	switch tt := t.(type) {
	case string:
		names = []string{tt}
	case []any:
		for _, n := range tt {
			if s, ok := n.(string); ok {
				names = append(names, s)
			}
		}
	default:
		return nil // malformed schema type; not this validator's job to police
	}
	for _, name := range names {
		if matchesType(name, v) {
			return nil
		}
	}
	return fmt.Errorf("%s: value does not match type %v", path, names)
}

func matchesType(name string, v any) bool {
	switch name {
	case "null":
		return v == nil
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		return !strings.ContainsAny(n.String(), ".eE")
	case "number":
		_, ok := v.(json.Number)
		return ok
	default:
		return false
	}
}

func enumContains(enum []any, v any) bool {
	for _, e := range enum {
		if deepEqualJSON(e, v) {
			return true
		}
	}
	return false
}

// deepEqualJSON compares two values decoded from JSON (schema literal vs.
// instance value) for equality, treating json.Number and plain numeric
// literals uniformly.
func deepEqualJSON(a, b any) bool {
	an, aIsNum := a.(json.Number)
	bn, bIsNum := b.(json.Number)
	if aIsNum || bIsNum {
		if !aIsNum {
			af, ok := a.(float64)
			if !ok {
				return false
			}
			an = json.Number(fmt.Sprintf("%v", af))
		}
		if !bIsNum {
			bf, ok := b.(float64)
			if !ok {
				return false
			}
			bn = json.Number(fmt.Sprintf("%v", bf))
		}
		return an.String() == bn.String()
	}
	return a == b
}
