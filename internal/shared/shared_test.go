package shared_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
)

func TestDefaultPagination(t *testing.T) {
	p := shared.DefaultPagination()
	if p.Page != 1 || p.Limit != 100 || p.OrderBy != "created_at" || p.Order != "DESC" {
		t.Errorf("unexpected defaults: %+v", p)
	}
}

func TestPaginationOffset(t *testing.T) {
	tests := []struct {
		page, limit, want int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{3, 25, 50},
		{1, 100, 0},
	}
	for _, tc := range tests {
		p := shared.Pagination{Page: tc.page, Limit: tc.limit}
		if got := p.Offset(); got != tc.want {
			t.Errorf("page=%d limit=%d: offset=%d, want %d", tc.page, tc.limit, got, tc.want)
		}
	}
}

func TestParsePagination(t *testing.T) {
	defaults := shared.DefaultPagination()
	allowed := []string{"created_at", "name"}

	tests := []struct {
		name    string
		query   string
		page    int
		limit   int
		orderBy string
		order   string
		hasSD   bool
		hasED   bool
	}{
		{"empty uses defaults", "", 1, 100, "created_at", "DESC", false, false},
		{"custom page and limit", "page=2&limit=50", 2, 50, "created_at", "DESC", false, false},
		{"limit clamped to 500", "limit=9999", 1, 500, "created_at", "DESC", false, false},
		{"order_by in allowlist", "order_by=name", 1, 100, "name", "DESC", false, false},
		{"order_by not in allowlist is ignored", "order_by=id%3BDROP+TABLE--", 1, 100, "created_at", "DESC", false, false},
		{"order case insensitive", "order=asc", 1, 100, "created_at", "ASC", false, false},
		{"invalid order is ignored", "order=SIDEWAYS", 1, 100, "created_at", "DESC", false, false},
		{"dates parsed", "start_date=2024-01-01T00:00:00Z&end_date=2024-12-31T23:59:59Z", 1, 100, "created_at", "DESC", true, true},
		{"invalid date ignored", "start_date=not-a-date", 1, 100, "created_at", "DESC", false, false},
		{"page less than 1 ignored", "page=0", 1, 100, "created_at", "DESC", false, false},
		{"page negative ignored", "page=-5", 1, 100, "created_at", "DESC", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			p := shared.ParsePagination(r, defaults, allowed)

			if p.Page != tc.page {
				t.Errorf("page: got %d, want %d", p.Page, tc.page)
			}
			if p.Limit != tc.limit {
				t.Errorf("limit: got %d, want %d", p.Limit, tc.limit)
			}
			if p.OrderBy != tc.orderBy {
				t.Errorf("order_by: got %q, want %q", p.OrderBy, tc.orderBy)
			}
			if p.Order != tc.order {
				t.Errorf("order: got %q, want %q", p.Order, tc.order)
			}
			if tc.hasSD && p.StartDate == nil {
				t.Error("start_date: expected non-nil, got nil")
			}
			if !tc.hasSD && p.StartDate != nil {
				t.Errorf("start_date: expected nil, got %v", p.StartDate)
			}
			if tc.hasED && p.EndDate == nil {
				t.Error("end_date: expected non-nil, got nil")
			}
			if !tc.hasED && p.EndDate != nil {
				t.Errorf("end_date: expected nil, got %v", p.EndDate)
			}
		})
	}
}

func TestNewPagedResult(t *testing.T) {
	data := []string{"a", "b", "c"}
	p := shared.Pagination{Page: 2, Limit: 3, OrderBy: "name", Order: "ASC"}

	result := shared.NewPagedResult(data, 7, p)

	if len(result.Data) != 3 {
		t.Errorf("data len: got %d, want 3", len(result.Data))
	}
	if result.Pagination.Total != 7 {
		t.Errorf("total: got %d, want 7", result.Pagination.Total)
	}
	if result.Pagination.TotalPages != 3 {
		t.Errorf("total_pages: got %d, want 3 (ceil(7/3))", result.Pagination.TotalPages)
	}
	if result.Pagination.Page != 2 {
		t.Errorf("page: got %d, want 2", result.Pagination.Page)
	}
	if result.Pagination.Limit != 3 {
		t.Errorf("limit: got %d, want 3", result.Pagination.Limit)
	}
	if result.Pagination.Sort != "name" {
		t.Errorf("sort: got %q, want name", result.Pagination.Sort)
	}
	if result.Pagination.SortDir != "ASC" {
		t.Errorf("sort_dir: got %q, want ASC", result.Pagination.SortDir)
	}
}

func TestNewPagedResult_ZeroTotal(t *testing.T) {
	result := shared.NewPagedResult([]int{}, 0, shared.DefaultPagination())
	if result.Pagination.TotalPages != 1 {
		t.Errorf("expected TotalPages=1 for zero total, got %d", result.Pagination.TotalPages)
	}
}

func TestNewPagedResult_ExactMultiple(t *testing.T) {
	p := shared.Pagination{Page: 1, Limit: 5, OrderBy: "created_at", Order: "DESC"}
	result := shared.NewPagedResult([]int{1, 2, 3, 4, 5}, 10, p)
	if result.Pagination.TotalPages != 2 {
		t.Errorf("expected TotalPages=2 for 10/5, got %d", result.Pagination.TotalPages)
	}
}
