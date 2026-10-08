package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"time"
)

// DecodeError describes a safe client-facing JSON request failure.
type DecodeError struct {
	Status  int
	Message string
}

func (e *DecodeError) Error() string { return e.Message }

// DecodeJSON requires application/json, rejects unknown fields and trailing JSON,
// and reports MaxBytesReader overflow as 413 Payload Too Large.
func DecodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &DecodeError{Status: http.StatusUnsupportedMediaType, Message: "content type must be application/json"}
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &DecodeError{Status: http.StatusRequestEntityTooLarge, Message: "request body is too large"}
		}
		return &DecodeError{Status: http.StatusBadRequest, Message: "request body contains invalid JSON"}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &DecodeError{Status: http.StatusRequestEntityTooLarge, Message: "request body is too large"}
		}
		return &DecodeError{Status: http.StatusBadRequest, Message: "request body must contain one JSON value"}
	}
	return nil
}

// EncodeJSON writes a JSON response and converts time.Time values to RFC 3339 UTC.
// Initialize collection fields before encoding when they must appear as [] or {}.
func EncodeJSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	normalized := normalizeTimes(reflect.ValueOf(value))
	var output any
	if normalized.IsValid() {
		output = normalized.Interface()
	}
	if err := json.NewEncoder(w).Encode(output); err != nil {
		return fmt.Errorf("encode JSON response: %w", err)
	}
	return nil
}

// Timestamp formats time as RFC 3339 UTC.
func Timestamp(value time.Time) string { return value.UTC().Format(time.RFC3339) }

func normalizeTimes(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		return reflect.ValueOf(value.Interface().(time.Time).UTC())
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		normalized := normalizeTimes(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(normalized)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(normalizeTimes(value.Elem()))
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath == "" {
				result.Field(i).Set(normalizeTimes(value.Field(i)))
			}
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(normalizeTimes(value.Index(i)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(normalizeTimes(value.Index(i)))
		}
		return result
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result.SetMapIndex(iter.Key(), normalizeTimes(iter.Value()))
		}
		return result
	default:
		return value
	}
}
