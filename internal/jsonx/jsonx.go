package jsonx

import (
	"encoding/json"
	"io"
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

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

func Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Normalize(v))
}

func Compact(v any) ([]byte, error) {
	return json.Marshal(Normalize(v))
}
