package catalog

import (
	"context"
	"math"
	"strings"

	"github.com/google/uuid"
)

type ListInput struct {
	Search   string
	SortBy   string
	Order    string
	Page     int
	PageSize int
	OwnerID  string
}

type ListResult struct {
	Items    []FileListItem `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, input ListInput) (*ListResult, error) {
	input.Search = strings.TrimSpace(input.Search)
	input.Order = strings.TrimSpace(input.Order)
	input.SortBy = strings.TrimSpace(input.SortBy)
	input.Order = strings.ToLower(input.Order)
	input.SortBy = strings.ToLower(input.SortBy)

	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = 20
	}
	if input.SortBy == "" {
		input.SortBy = "name"
	}
	if input.Order == "" {
		input.Order = "asc"
	}
	if input.Page < 1 || input.PageSize < 1 || input.PageSize > 100 {
		return nil, ErrInvalidInput
	}

	if input.SortBy != "name" && input.SortBy != "owner" && input.SortBy != "downloads" {
		return nil, ErrInvalidInput
	}

	if input.Order != "asc" && input.Order != "desc" {
		return nil, ErrInvalidInput
	}

	if input.Page-1 > math.MaxInt/input.PageSize {
		return nil, ErrInvalidInput
	}

	if input.OwnerID != "" {
		if len(input.OwnerID) != 36 {
			return nil, ErrInvalidInput
		}
		if _, err := uuid.Parse(input.OwnerID); err != nil {
			return nil, ErrInvalidInput
		}
	}

	options := ListOptions{
		Search:  input.Search,
		SortBy:  input.SortBy,
		Desc:    input.Order == "desc",
		Limit:   input.PageSize,
		Offset:  (input.Page - 1) * input.PageSize,
		OwnerID: input.OwnerID,
	}

	items, total, err := s.repository.List(ctx, options)
	if err != nil {
		return nil, err
	}

	res := ListResult{
		Items:    items,
		Total:    total,
		Page:     input.Page,
		PageSize: input.PageSize,
	}

	return &res, nil
}
