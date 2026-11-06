// Package jsonmerge preserves server fields unknown to an older API model.
package jsonmerge

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// Option configures preservation for API collections with explicit identity semantics.
type Option func(*mergeOptions)

type mergeOptions struct {
	positionalElements map[reflect.Type]bool
}

// WithPositionalArrayElements retains unknown fields by index for the given
// element types, including when an array grows or shrinks. Use only when the
// API converter itself updates these collections by position.
func WithPositionalArrayElements(elements ...any) Option {
	return func(options *mergeOptions) {
		for _, element := range elements {
			model := reflect.TypeOf(element)
			if model != nil {
				options.positionalElements[model] = true
			}
		}
	}
}

// MarshalPreservingUnknown marshals value while retaining unknown fields from
// original. Known fields omitted by value remain omitted, so explicit resets
// cannot resurrect old values. Callers with MarshalJSON must pass an alias.
func MarshalPreservingUnknown(original json.RawMessage, value any, options ...Option) ([]byte, error) {
	settings := &mergeOptions{positionalElements: make(map[reflect.Type]bool)}
	for _, option := range options {
		option(settings)
	}
	current, err := json.Marshal(value)
	if err != nil || len(original) == 0 {
		return current, err
	}
	var before, after any
	if err := decode(original, &before); err != nil {
		return nil, err
	}
	if err := decode(current, &after); err != nil {
		return nil, err
	}
	return json.Marshal(mergeUnknown(before, after, reflect.TypeOf(value), settings))
}

func decode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func mergeUnknown(before, after any, model reflect.Type, settings *mergeOptions) any {
	if model == nil || after == nil {
		return after
	}
	for model.Kind() == reflect.Pointer {
		model = model.Elem()
	}
	if model.Kind() == reflect.Slice || model.Kind() == reflect.Array {
		oldItems, oldOK := before.([]any)
		newItems, newOK := after.([]any)
		if oldOK && newOK {
			return mergeArray(oldItems, newItems, model.Elem(), settings)
		}
		return after
	}
	oldObject, oldOK := before.(map[string]any)
	newObject, newOK := after.(map[string]any)
	if !oldOK || !newOK {
		return after
	}
	if model.Kind() == reflect.Map {
		for key, value := range newObject {
			newObject[key] = mergeUnknown(oldObject[key], value, model.Elem(), settings)
		}
		return newObject
	}
	if model.Kind() != reflect.Struct {
		return after
	}
	fields := jsonFields(model)
	for key, value := range oldObject {
		if _, known := fields[key]; !known {
			if _, replaced := newObject[key]; !replaced {
				newObject[key] = value
			}
		}
	}
	for key, value := range newObject {
		if field, known := fields[key]; known {
			newObject[key] = mergeUnknown(oldObject[key], value, field, settings)
		}
	}
	return newObject
}

func jsonFields(model reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for i := 0; i < model.NumField(); i++ {
		field := model.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" && field.Anonymous {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				for key, value := range jsonFields(embedded) {
					fields[key] = value
				}
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func mergeArray(before, after []any, model reflect.Type, settings *mergeOptions) []any {
	if settings.positionalElements[model] {
		for i := 0; i < len(before) && i < len(after); i++ {
			after[i] = mergeUnknown(before[i], after[i], model, settings)
		}
		return after
	}
	byIdentity := make(map[string][]any)
	for _, item := range before {
		if key := identity(item); key != "" {
			byIdentity[key] = append(byIdentity[key], item)
		}
	}
	for i, item := range after {
		key := identity(item)
		if candidates := byIdentity[key]; key != "" && len(candidates) > 0 {
			after[i] = mergeUnknown(candidates[0], item, model, settings)
			byIdentity[key] = candidates[1:]
		} else if key == "" && len(before) == len(after) && identity(before[i]) == "" {
			after[i] = mergeUnknown(before[i], item, model, settings)
		}
	}
	return after
}

func identity(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	keys := []string{}
	if kind, ok := object["kind"].(string); ok && kind != "" {
		keys = []string{"apiVersion", "kind", "namespace", "name", "uid"}
	} else {
		for _, candidate := range [][]string{{"name", "namespace"}, {"deviceName"}, {"key", "operator"}} {
			if value, ok := object[candidate[0]].(string); ok && value != "" {
				keys = candidate
				break
			}
		}
	}
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i], _ = object[key].(string)
	}
	if keys[0] == "apiVersion" {
		parts = append(parts, selectorIdentity(object["labelSelector"]))
	}
	encoded, _ := json.Marshal(parts)
	return keys[0] + string(encoded)
}

type selectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type selector struct {
	MatchLabels      map[string]string     `json:"matchLabels,omitempty"`
	MatchExpressions []selectorRequirement `json:"matchExpressions,omitempty"`
}

func selectorIdentity(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var normalized selector
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return ""
	}
	for i := range normalized.MatchExpressions {
		sort.Strings(normalized.MatchExpressions[i].Values)
	}
	sort.SliceStable(normalized.MatchExpressions, func(i, j int) bool {
		left, _ := json.Marshal(normalized.MatchExpressions[i])
		right, _ := json.Marshal(normalized.MatchExpressions[j])
		return string(left) < string(right)
	})
	encoded, _ = json.Marshal(normalized)
	return string(encoded)
}
