package jsonx

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"time"
)

var (
	timeType     = reflect.TypeOf(time.Time{})
	durationType = reflect.TypeOf(time.Duration(0))
)

func Normalize(v any) any {
	if v == nil {
		return []any{}
	}
	return normalizeValue(reflect.ValueOf(v)).Interface()
}

func normalizeValue(rv reflect.Value) reflect.Value {
	if !rv.IsValid() {
		return rv
	}
	if rv.Type() == durationType {
		return reflect.ValueOf(DurationMS(time.Duration(rv.Int())))
	}
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return rv
		}
		return normalizeValue(rv.Elem())
	case reflect.Pointer:
		if rv.IsNil() {
			if rv.Type().Elem().Kind() == reflect.Slice {
				return reflect.MakeSlice(rv.Type().Elem(), 0, 0)
			}
			return rv
		}
		elem := normalizeValue(rv.Elem())
		if !elem.IsValid() || !elem.Type().AssignableTo(rv.Type().Elem()) {
			return rv
		}
		out := reflect.New(rv.Type().Elem())
		out.Elem().Set(elem)
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return reflect.MakeSlice(rv.Type(), 0, 0)
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			nv := normalizeValue(rv.Index(i))
			if nv.IsValid() && nv.Type().AssignableTo(rv.Type().Elem()) {
				out.Index(i).Set(nv)
			} else {
				out.Index(i).Set(rv.Index(i))
			}
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		for _, k := range rv.MapKeys() {
			nv := normalizeValue(rv.MapIndex(k))
			if nv.IsValid() && nv.Type().AssignableTo(rv.Type().Elem()) {
				out.SetMapIndex(k, nv)
			} else {
				out.SetMapIndex(k, rv.MapIndex(k))
			}
		}
		return out
	case reflect.Struct:
		if rv.Type() == timeType {
			return rv
		}
		if structHasDuration(rv.Type()) {
			return normalizeDurationStruct(rv)
		}
		out := reflect.New(rv.Type()).Elem()
		out.Set(rv)
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			if !out.Field(i).CanSet() || rt.Field(i).PkgPath != "" {
				continue
			}
			nv := normalizeValue(out.Field(i))
			if nv.IsValid() && nv.Type().AssignableTo(out.Field(i).Type()) {
				out.Field(i).Set(nv)
			}
		}
		return out
	default:
		return rv
	}
}

func structHasDuration(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Type == durationType {
			return true
		}
		if f.Type.Kind() == reflect.Struct && f.Type != timeType && structHasDuration(f.Type) {
			return true
		}
	}
	return false
}

func jsonFieldName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name := f.Name
	if tag != "" {
		n := strings.Split(tag, ",")[0]
		if n != "" {
			name = n
		}
	}
	return name, true
}

func normalizeDurationStruct(rv reflect.Value) reflect.Value {
	m := map[string]any{}
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name, ok := jsonFieldName(f)
		if !ok {
			continue
		}
		m[name] = normalizeValue(rv.Field(i)).Interface()
	}
	return reflect.ValueOf(m)
}

func Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Normalize(v))
}

func Compact(v any) ([]byte, error) {
	return json.Marshal(Normalize(v))
}
