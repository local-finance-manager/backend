package shared

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxLimit = 500

// Pagination holds all parameters for a paginated query, including optional date range filters.
type Pagination struct {
	Page      int
	Limit     int
	OrderBy   string
	Order     string     // "ASC" or "DESC"
	StartDate *time.Time // created_at >= StartDate
	EndDate   *time.Time // created_at <= EndDate
}

// DefaultPagination returns a sensible default pagination.
func DefaultPagination() Pagination {
	return Pagination{Page: 1, Limit: 100, OrderBy: "created_at", Order: "DESC"}
}

// Offset computes the SQL OFFSET value.
func (p Pagination) Offset() int { return (p.Page - 1) * p.Limit }

// ParsePagination reads pagination params from query string.
// allowedOrderBy is an allowlist of safe column names for ORDER BY.
func ParsePagination(r *http.Request, defaults Pagination, allowedOrderBy []string) Pagination {
	p := defaults
	q := r.URL.Query()

	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			p.Page = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			if n > maxLimit {
				n = maxLimit
			}
			p.Limit = n
		}
	}
	if v := q.Get("order_by"); v != "" {
		for _, allowed := range allowedOrderBy {
			if v == allowed {
				p.OrderBy = v
				break
			}
		}
	}
	if v := strings.ToUpper(q.Get("order")); v == "ASC" || v == "DESC" {
		p.Order = v
	}
	if v := q.Get("start_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.StartDate = &t
		}
	}
	if v := q.Get("end_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.EndDate = &t
		}
	}
	return p
}

// PagedResult is a generic paginated response container.
type PagedResult[T any] struct {
	Data       []T      `json:"data"`
	Pagination PagedMeta `json:"pagination"`
}

// PagedMeta echoes pagination params back to the caller.
type PagedMeta struct {
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	Total      int    `json:"total"`
	TotalPages int    `json:"total_pages"`
	Sort       string `json:"sort"`
	SortDir    string `json:"sort_dir"`
}

// NewPagedResult builds a PagedResult from a data slice and total count.
func NewPagedResult[T any](data []T, total int, p Pagination) PagedResult[T] {
	totalPages := 1
	if p.Limit > 0 && total > 0 {
		totalPages = (total + p.Limit - 1) / p.Limit
	}
	return PagedResult[T]{
		Data: data,
		Pagination: PagedMeta{
			Page:       p.Page,
			Limit:      p.Limit,
			Total:      total,
			TotalPages: totalPages,
			Sort:       p.OrderBy,
			SortDir:    p.Order,
		},
	}
}
