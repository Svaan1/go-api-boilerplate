// Package apiquery parses pagination, sorting, and filtering query parameters.
package apiquery

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/svaan1/go-api-boilerplate/internal/problem"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// Pagination describes one-based page selection and its zero-based row offset.
type Pagination struct {
	Page     int
	PageSize int
	Offset   int
}

// SortField is one allowlisted sort key. Descending selects descending order.
type SortField struct {
	Field      string
	Descending bool
}

// ParsePagination reads page and page_size, applying documented defaults and limits.
func ParsePagination(values url.Values) (Pagination, []problem.ValidationError) {
	page, pageErrors := parsePositiveInt(values, "page", defaultPage, 0)
	pageSize, sizeErrors := parsePositiveInt(values, "page_size", defaultPageSize, maxPageSize)
	errors := append(pageErrors, sizeErrors...)
	if len(errors) != 0 {
		return Pagination{}, errors
	}

	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/pageSize {
		return Pagination{}, []problem.ValidationError{{
			Field:   "page",
			Code:    "invalid_value",
			Message: "page and page_size produce an offset that is too large",
		}}
	}
	return Pagination{Page: page, PageSize: pageSize, Offset: (page - 1) * pageSize}, nil
}

// ParseSort parses sort=field,-field against allowedFields. Field names are
// returned only after exact allowlist matching; callers must still bind values
// and quote or otherwise safely render identifiers for their query language.
func ParseSort(values url.Values, allowedFields []string) ([]SortField, []problem.ValidationError) {
	raw, present, validationErrors := oneValue(values, "sort")
	if len(validationErrors) != 0 {
		return nil, validationErrors
	}
	if !present {
		return nil, nil
	}

	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}

	parts := strings.Split(raw, ",")
	fields := make([]SortField, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, invalidValue("sort", "sort must contain non-empty fields")
		}

		descending := strings.HasPrefix(part, "-")
		field := part
		if strings.HasPrefix(part, "-") || strings.HasPrefix(part, "+") {
			field = part[1:]
		}
		if field == "" {
			return nil, invalidValue("sort", "sort fields must not be empty")
		}
		if _, ok := allowed[field]; !ok {
			return nil, []problem.ValidationError{{
				Field:   "sort",
				Code:    "unsupported_field",
				Message: "sort field is not supported",
			}}
		}
		if _, exists := seen[field]; exists {
			return nil, invalidValue("sort", "sort field must not be repeated")
		}
		seen[field] = struct{}{}
		fields = append(fields, SortField{Field: field, Descending: descending})
	}
	return fields, nil
}

// ParseFilters returns query filters whose keys are in allowedFields. Pagination
// and sort parameters are reserved and ignored. Repeated identical filter
// values are accepted; conflicting repeated values are rejected.
func ParseFilters(values url.Values, allowedFields []string) (map[string]string, []problem.ValidationError) {
	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}

	filters := make(map[string]string)
	var validationErrors []problem.ValidationError
	fields := make([]string, 0, len(values))
	for field := range values {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		fieldValues := values[field]
		if field == "page" || field == "page_size" || field == "sort" {
			continue
		}
		if _, ok := allowed[field]; !ok {
			validationErrors = append(validationErrors, problem.ValidationError{
				Field:   field,
				Code:    "unsupported_field",
				Message: "filter field is not supported",
			})
			continue
		}
		if len(fieldValues) == 0 {
			validationErrors = append(validationErrors, problem.ValidationError{
				Field:   field,
				Code:    "invalid_value",
				Message: "filter value is required",
			})
			continue
		}
		value := fieldValues[0]
		conflict := false
		for _, candidate := range fieldValues[1:] {
			if candidate != value {
				conflict = true
				break
			}
		}
		if conflict {
			validationErrors = append(validationErrors, problem.ValidationError{
				Field:   field,
				Code:    "conflicting_values",
				Message: "filter must not have conflicting values",
			})
			continue
		}
		filters[field] = value
	}
	if len(validationErrors) != 0 {
		return nil, validationErrors
	}
	return filters, nil
}

func parsePositiveInt(values url.Values, field string, fallback, maximum int) (int, []problem.ValidationError) {
	raw, present, validationErrors := oneValue(values, field)
	if len(validationErrors) != 0 {
		return 0, validationErrors
	}
	if !present {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	maxInt := int64(^uint(0) >> 1)
	if err != nil || parsed < 1 || (maximum != 0 && parsed > int64(maximum)) || parsed > maxInt {
		message := "value must be a positive integer"
		if maximum != 0 {
			message = "value must be between 1 and " + strconv.Itoa(maximum)
		}
		return 0, invalidValue(field, message)
	}
	return int(parsed), nil
}

func oneValue(values url.Values, field string) (string, bool, []problem.ValidationError) {
	fieldValues, present := values[field]
	if !present {
		return "", false, nil
	}
	if len(fieldValues) == 0 {
		return "", true, invalidValue(field, "value is required")
	}
	value := fieldValues[0]
	for _, candidate := range fieldValues[1:] {
		if candidate != value {
			return "", true, []problem.ValidationError{{
				Field:   field,
				Code:    "conflicting_values",
				Message: "parameter must not have conflicting values",
			}}
		}
	}
	return value, true, nil
}

func invalidValue(field, message string) []problem.ValidationError {
	return []problem.ValidationError{{Field: field, Code: "invalid_value", Message: message}}
}
