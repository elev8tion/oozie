package tasks

import (
	"context"
	"errors"
	"strings"
)

var ErrBlankTitle = errors.New("task title cannot be blank")

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context) ([]Task, error) {
	return s.repo.List(ctx)
}

func (s *Service) Create(ctx context.Context, title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrBlankTitle
	}
	return s.repo.Create(ctx, title)
}

func (s *Service) Complete(ctx context.Context, id int64) (Task, error) {
	return s.repo.Complete(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
