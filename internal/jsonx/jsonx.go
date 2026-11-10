package jsonx

import (
	"encoding/json"
	"io"
	"reflect"
)

func Normalize(v any) any {
	if v == nil {
		return []any{}
	}
	return normalizeValue(reflect.ValueOf(v)).Interface()
}

func normalizeValue(rv reflect.Value) reflect.Value {
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			if rv.Kind() == reflect.Pointer && rv.Type().Elem().Kind() == reflect.Slice {
				return reflect.MakeSlice(rv.Type().Elem(), 0, 0)
			}
			return rv
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return reflect.MakeSlice(rv.Type(), 0, 0)
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out.Index(i).Set(normalizeValue(rv.Index(i)))
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		for _, k := range rv.MapKeys() {
			rv.SetMapIndex(k, normalizeValue(rv.MapIndex(k)))
		}
		return rv
	case reflect.Struct:
		out := reflect.New(rv.Type()).Elem()
		out.Set(rv)
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			if !out.Field(i).CanSet() {
				continue
			}
			if rt.Field(i).PkgPath != "" {
				continue
			}
			out.Field(i).Set(normalizeValue(out.Field(i)))
		}
		return out
	default:
		return rv
	}
}

func Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Normalize(v))
}

func Compact(v any) ([]byte, error) {
	return json.Marshal(Normalize(v))
}
