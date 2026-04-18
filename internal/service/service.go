package service

import (
	"context"

	"github.com/forechoandlook/gtask/internal/config"
	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/store"
	"github.com/forechoandlook/gtask/internal/syncer"
)

type Service interface {
	AddTask(ctx context.Context, in store.AddInput) (model.Task, error)
	GetTask(ctx context.Context, id int64) (model.Task, error)
	ListTasks(ctx context.Context, includeCompleted bool) ([]model.Task, error)
	ListTasksFiltered(ctx context.Context, filter store.ListFilter) ([]model.Task, error)
	UpdateTask(ctx context.Context, in store.UpdateInput) (model.Task, error)
	DeleteTask(ctx context.Context, id int64) error
	Sync(ctx context.Context) (string, error)
}

type LocalService struct {
	Store *store.Store
	Cfg   config.Config
}

func (s *LocalService) AddTask(ctx context.Context, in store.AddInput) (model.Task, error) {
	return s.Store.AddTask(ctx, in)
}

func (s *LocalService) GetTask(ctx context.Context, id int64) (model.Task, error) {
	return s.Store.GetTask(ctx, id)
}

func (s *LocalService) ListTasks(ctx context.Context, includeCompleted bool) ([]model.Task, error) {
	return s.Store.ListTasks(ctx, includeCompleted)
}

func (s *LocalService) ListTasksFiltered(ctx context.Context, filter store.ListFilter) ([]model.Task, error) {
	return s.Store.ListTasksFiltered(ctx, filter)
}

func (s *LocalService) UpdateTask(ctx context.Context, in store.UpdateInput) (model.Task, error) {
	return s.Store.UpdateTask(ctx, in)
}

func (s *LocalService) DeleteTask(ctx context.Context, id int64) error {
	return s.Store.DeleteTask(ctx, id)
}

func (s *LocalService) Sync(ctx context.Context) (string, error) {
	return syncer.New(s.Cfg, s.Store).Sync(ctx)
}

func New(cfg config.Config, st *store.Store) Service {
	return &LocalService{
		Store: st,
		Cfg:   cfg,
	}
}
