package api

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/flags"
	"github.com/nodelistdb/internal/pingtrace"
	"github.com/nodelistdb/internal/ratelimit"
	"github.com/nodelistdb/internal/storage"
	"gopkg.in/yaml.v3"
)

// TestSchemasMatchTheirTypes holds every named schema in openapi.yaml against
// the Go type the handlers actually encode. The spec once described a Node
// with six boolean fields that had been removed a year earlier, a max_speed
// that was a string, and a SoftwareDistribution whose only list did not
// exist; a hand-edited schema drifts silently, so each one is derived from
// its struct here and diffed on every run.
//
// A property is compared by name and by JSON type class (integer, number,
// string, boolean, array, object), recursively through arrays, maps and
// $ref targets. nullable and omitempty are not compared: the encoder makes
// those decisions per value, not per type.
func TestSchemasMatchTheirTypes(t *testing.T) {
	// The schemas that are struct-backed. The few the handlers assemble from
	// maps (AddressEnvelope, NodelistInfo, the inline response objects) are
	// pinned by TestResponsesMatchTheSpec instead.
	types := map[string]reflect.Type{
		"Node":                     reflect.TypeOf(database.Node{}),
		"NodeChange":               reflect.TypeOf(database.NodeChange{}),
		"NetworkStats":             reflect.TypeOf(database.NetworkStats{}),
		"RegionInfo":               reflect.TypeOf(database.RegionInfo{}),
		"NetInfo":                  reflect.TypeOf(database.NetInfo{}),
		"Point":                    reflect.TypeOf(database.Point{}),
		"PointlistFile":            reflect.TypeOf(database.PointlistFile{}),
		"PointlistSource":          reflect.TypeOf(storage.PointlistSourceInfo{}),
		"SysopInfo":                reflect.TypeOf(storage.SysopInfo{}),
		"NetworkInfo":              reflect.TypeOf(storage.DomainInfo{}),
		"SoftwareDistribution":     reflect.TypeOf(storage.SoftwareDistribution{}),
		"SoftwareTypeStats":        reflect.TypeOf(storage.SoftwareTypeStats{}),
		"SoftwareVersionStats":     reflect.TypeOf(storage.SoftwareVersionStats{}),
		"OSStats":                  reflect.TypeOf(storage.OSStats{}),
		"GeoHostingDistribution":   reflect.TypeOf(storage.GeoHostingDistribution{}),
		"CountryStats":             reflect.TypeOf(storage.CountryStats{}),
		"ProviderStats":            reflect.TypeOf(storage.ProviderStats{}),
		"PingTraceSummary":         reflect.TypeOf(storage.PingTraceSummary{}),
		"PingNodeSummary":          reflect.TypeOf(storage.PingNodeSummary{}),
		"TracerStat":               reflect.TypeOf(storage.TracerStat{}),
		"RobotStat":                reflect.TypeOf(storage.RobotStat{}),
		"Ping":                     reflect.TypeOf(pingtrace.Ping{}),
		"PingReply":                reflect.TypeOf(storage.PingReplyRow{}),
		"Hop":                      reflect.TypeOf(pingtrace.Hop{}),
		"PSTNNode":                 reflect.TypeOf(storage.PSTNNode{}),
		"PSTNDeadNode":             reflect.TypeOf(storage.PSTNDeadNode{}),
		"HealthStatus":             reflect.TypeOf(HealthStatus{}),
		"VersionInfo":              reflect.TypeOf(VersionInfo{}),
		"DatabaseHealth":           reflect.TypeOf(DatabaseHealth{}),
		"CacheHealth":              reflect.TypeOf(CacheHealth{}),
		"FTPHealth":                reflect.TypeOf(FTPHealth{}),
		"NodeCountInfo":            reflect.TypeOf(NodeCountInfo{}),
		"ModemTestResultRequest":   reflect.TypeOf(ModemTestResultRequest{}),
		"LineStatsRequest":         reflect.TypeOf(LineStatsRequest{}),
		"AudioCodesCDRRequest":     reflect.TypeOf(AudioCodesCDRRequest{}),
		"AsteriskCDRRequest":       reflect.TypeOf(AsteriskCDRRequest{}),
		"NodeTestResult":           reflect.TypeOf(storage.NodeTestResult{}),
		"NodeReachabilityStats":    reflect.TypeOf(storage.NodeReachabilityStats{}),
		"ReachabilityTrend":        reflect.TypeOf(storage.ReachabilityTrend{}),
		"CacheStats":               reflect.TypeOf(CacheStats{}),
		"FTPStats":                 reflect.TypeOf(FTPStats{}),
		"RateLimitStats":           reflect.TypeOf(ratelimit.Stats{}),
		"NodeSearchFilter":         reflect.TypeOf(nodeFilterEcho{}),
		"PointSearchFilter":        reflect.TypeOf(pointFilterEcho{}),
		"ReachabilitySearchFilter": reflect.TypeOf(reachabilityFilterEcho{}),
		"TimelineEvent":            reflect.TypeOf(timelineEvent{}),
		"FlagInfo":                 reflect.TypeOf(flags.FlagInfo{}),
		"StatusMessage":            reflect.TypeOf(statusMessage{}),
		"Error":                    reflect.TypeOf(errorBody{}),
	}

	spec := loadSpec(t)
	for name := range spec.Components.Schemas {
		if _, ok := types[name]; !ok && !mapBackedSchemas[name] {
			t.Errorf("schema %s is in openapi.yaml but no Go type is registered for it here", name)
		}
	}
	for name, typ := range types {
		schema, ok := spec.Components.Schemas[name]
		if !ok {
			t.Errorf("schema %s is registered here but missing from openapi.yaml", name)
			continue
		}
		checkStruct(t, spec, types, name, schema, typ)
	}
}

// checkStruct compares one struct type with one object schema, property by
// property, and recurses into arrays, maps and referenced schemas so that a
// change to an element type or to which record a field holds is caught, not
// only a change at the top level.
func checkStruct(t *testing.T, spec *specDoc, types map[string]reflect.Type, name string, schema specSchema, typ reflect.Type) {
	t.Helper()
	want := jsonFields(typ)
	for prop, ps := range schema.Properties {
		ft, ok := want[prop]
		if !ok {
			t.Errorf("%s: the schema lists %q but the struct never emits it", name, prop)
			continue
		}
		checkValue(t, spec, types, name+"."+prop, ps, ft)
	}
	for prop := range want {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("%s: the struct emits %q but the schema lacks it", name, prop)
		}
	}
}

// checkValue compares one schema with one Go type.
func checkValue(t *testing.T, spec *specDoc, types map[string]reflect.Type, where string, s specSchema, typ reflect.Type) {
	t.Helper()
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	switch {
	case s.Ref != "":
		refName := s.Ref[strings.LastIndex(s.Ref, "/")+1:]
		target, registered := types[refName]
		switch {
		case !registered && !mapBackedSchemas[refName]:
			t.Errorf("%s: refers to schema %s, which no Go type is registered for", where, refName)
		case registered && target != typ:
			t.Errorf("%s: refers to schema %s (%s), but the field is %s", where, refName, target, typ)
		}
	case len(s.AllOf) > 0:
		checkValue(t, spec, types, where, mergeAllOf(spec, s.AllOf), typ)
	case s.Type == "array":
		if classOf(typ) != "array" {
			t.Errorf("%s: schema says array, the struct emits %s", where, classOf(typ))
			return
		}
		if s.Items != nil {
			checkValue(t, spec, types, where+"[]", *s.Items, typ.Elem())
		}
	case s.Type == "object" && typ.Kind() == reflect.Map:
		if inner, ok := additionalPropertiesSchema(s); ok {
			checkValue(t, spec, types, where+"{}", inner, typ.Elem())
		}
	case s.Type == "object" && typ.Kind() == reflect.Struct && typ != timeType:
		// An inline object with properties against a struct: compare them
		// too. Without properties (additionalProperties: true) anything goes.
		if len(s.Properties) > 0 {
			checkStruct(t, spec, types, where, s, typ)
		}
	default:
		if got := classOf(typ); s.Type != "" && got != s.Type {
			t.Errorf("%s: schema says %s, the struct emits %s", where, s.Type, got)
		}
	}
}

// mergeAllOf folds the parts of an allOf into one schema: the union of
// their properties, and the first $ref or type any part names. A $ref part
// is expanded so its properties take part in the union.
func mergeAllOf(spec *specDoc, parts []specSchema) specSchema {
	merged := specSchema{Properties: map[string]specSchema{}}
	for _, part := range parts {
		if part.Ref != "" {
			if len(parts) == 1 {
				return part
			}
			name := part.Ref[strings.LastIndex(part.Ref, "/")+1:]
			part = spec.Components.Schemas[name]
		}
		if merged.Type == "" {
			merged.Type = part.Type
		}
		for k, v := range part.Properties {
			merged.Properties[k] = v
		}
	}
	return merged
}

// additionalPropertiesSchema returns the value schema of a map-typed schema,
// or false when it is absent or the boolean form.
func additionalPropertiesSchema(s specSchema) (specSchema, bool) {
	m, ok := s.AdditionalProperties.(map[string]interface{})
	if !ok {
		return specSchema{}, false
	}
	raw, err := yaml.Marshal(m)
	if err != nil {
		return specSchema{}, false
	}
	var inner specSchema
	if err := yaml.Unmarshal(raw, &inner); err != nil {
		return specSchema{}, false
	}
	return inner, true
}

// mapBackedSchemas are assembled by the handlers from maps and inline
// objects rather than encoded from one struct.
var mapBackedSchemas = map[string]bool{
	"AddressEnvelope": true,
	"NodelistInfo":    true,
}

type specDoc struct {
	Paths      map[string]map[string]specOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]specSchema `yaml:"schemas"`
	} `yaml:"components"`
}

type specOperation struct {
	Parameters []specParameter         `yaml:"parameters"`
	Responses  map[string]specResponse `yaml:"responses"`
}

type specParameter struct {
	Ref  string `yaml:"$ref"`
	Name string `yaml:"name"`
	In   string `yaml:"in"`
}

type specResponse struct {
	Content map[string]struct {
		Schema specSchema `yaml:"schema"`
	} `yaml:"content"`
}

type specSchema struct {
	Ref                  string                `yaml:"$ref"`
	Type                 string                `yaml:"type"`
	Properties           map[string]specSchema `yaml:"properties"`
	Items                *specSchema           `yaml:"items"`
	AllOf                []specSchema          `yaml:"allOf"`
	AdditionalProperties interface{}           `yaml:"additionalProperties"`
}

func loadSpec(t *testing.T) *specDoc {
	t.Helper()
	body, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec specDoc
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatalf("parsing openapi.yaml: %v", err)
	}
	return &spec
}

var (
	timeType     = reflect.TypeOf(time.Time{})
	durationType = reflect.TypeOf(time.Duration(0))
	rawType      = reflect.TypeOf(json.RawMessage{})
)

// jsonFields returns the JSON keys a struct type encodes to and each key's
// Go type, following the encoding/json rules that matter here: tags, "-",
// embedded structs flattened, unexported fields skipped.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if f.Anonymous && name == "" {
				ft := f.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					walk(ft)
					continue
				}
			}
			if f.PkgPath != "" {
				continue // unexported
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
	}
	walk(t)
	return fields
}

func classOf(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return "string"
	case t == durationType:
		return "integer"
	case t == rawType:
		return "object"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "string" // []byte encodes as base64
		}
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	}
	return ""
}

// sortedKeys is a test helper for stable diagnostics.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
