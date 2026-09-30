package apiquery

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		values     url.Values
		expected   Pagination
		errorField string
	}{
		{name: "defaults", values: url.Values{}, expected: Pagination{Page: 1, PageSize: 20, Offset: 0}},
		{name: "first page", values: url.Values{"page": {"1"}, "page_size": {"1"}}, expected: Pagination{Page: 1, PageSize: 1, Offset: 0}},
		{name: "maximum page size", values: url.Values{"page": {"3"}, "page_size": {"100"}}, expected: Pagination{Page: 3, PageSize: 100, Offset: 200}},
		{name: "zero page", values: url.Values{"page": {"0"}}, errorField: "page"},
		{name: "negative page size", values: url.Values{"page_size": {"-1"}}, errorField: "page_size"},
		{name: "page size above maximum", values: url.Values{"page_size": {"101"}}, errorField: "page_size"},
		{name: "malformed page", values: url.Values{"page": {"one"}}, errorField: "page"},
		{name: "conflicting page values", values: url.Values{"page": {"2", "3"}}, errorField: "page"},
		{name: "same repeated page value", values: url.Values{"page": {"2", "2"}}, expected: Pagination{Page: 2, PageSize: 20, Offset: 20}},
		{name: "offset overflow", values: url.Values{"page": {"9223372036854775807"}, "page_size": {"100"}}, errorField: "page"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, validationErrors := ParsePagination(test.values)
			if test.errorField != "" {
				if len(validationErrors) == 0 || validationErrors[0].Field != test.errorField {
					t.Fatalf("errors = %#v, want validation error for %q", validationErrors, test.errorField)
				}
				return
			}
			if len(validationErrors) != 0 {
				t.Fatalf("unexpected validation errors: %#v", validationErrors)
			}
			if got != test.expected {
				t.Errorf("pagination = %#v, want %#v", got, test.expected)
			}
		})
	}
}

func TestParseSort(t *testing.T) {
	tests := []struct {
		name      string
		values    url.Values
		expected  []SortField
		errorCode string
	}{
		{name: "no sort", values: url.Values{}},
		{name: "ascending and descending fields", values: url.Values{"sort": {"name,+created_at,-id"}}, expected: []SortField{{Field: "name"}, {Field: "created_at"}, {Field: "id", Descending: true}}},
		{name: "unknown field", values: url.Values{"sort": {"name;drop table users"}}, errorCode: "unsupported_field"},
		{name: "empty field", values: url.Values{"sort": {"name,,id"}}, errorCode: "invalid_value"},
		{name: "duplicate field", values: url.Values{"sort": {"name,-name"}}, errorCode: "invalid_value"},
		{name: "conflicting repeated parameters", values: url.Values{"sort": {"name", "-id"}}, errorCode: "conflicting_values"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, validationErrors := ParseSort(test.values, []string{"name", "created_at", "id"})
			if test.errorCode != "" {
				if len(validationErrors) != 1 || validationErrors[0].Code != test.errorCode {
					t.Fatalf("errors = %#v, want one %q error", validationErrors, test.errorCode)
				}
				return
			}
			if len(validationErrors) != 0 {
				t.Fatalf("unexpected validation errors: %#v", validationErrors)
			}
			if !reflect.DeepEqual(got, test.expected) {
				t.Errorf("sort fields = %#v, want %#v", got, test.expected)
			}
		})
	}
}

func TestParseFilters(t *testing.T) {
	tests := []struct {
		name      string
		values    url.Values
		expected  map[string]string
		errorCode string
	}{
		{name: "empty filters", values: url.Values{"page": {"2"}, "sort": {"name"}}, expected: map[string]string{}},
		{name: "allowlisted filters", values: url.Values{"status": {"active"}, "owner": {"user-1"}, "page_size": {"25"}}, expected: map[string]string{"status": "active", "owner": "user-1"}},
		{name: "unknown field rejected", values: url.Values{"status;drop table users": {"active"}}, errorCode: "unsupported_field"},
		{name: "conflicting repeated values", values: url.Values{"status": {"active", "disabled"}}, errorCode: "conflicting_values"},
		{name: "same repeated value", values: url.Values{"status": {"active", "active"}}, expected: map[string]string{"status": "active"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, validationErrors := ParseFilters(test.values, []string{"status", "owner"})
			if test.errorCode != "" {
				if len(validationErrors) != 1 || validationErrors[0].Code != test.errorCode {
					t.Fatalf("errors = %#v, want one %q error", validationErrors, test.errorCode)
				}
				return
			}
			if len(validationErrors) != 0 {
				t.Fatalf("unexpected validation errors: %#v", validationErrors)
			}
			if !reflect.DeepEqual(got, test.expected) {
				t.Errorf("filters = %#v, want %#v", got, test.expected)
			}
		})
	}
}
