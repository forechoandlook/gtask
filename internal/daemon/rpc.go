package daemon

import (
	"context"

	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
)

type RPCServer struct {
	svc    service.Service
	notify func(title, msg string)
}

func (s *RPCServer) AddTask(in store.AddInput, out *model.Task) error {
	task, err := s.svc.AddTask(context.Background(), in)
	*out = task
	if err == nil {
		s.notify("gtask", "Added: " + task.Title)
	}
	return err
}

func (s *RPCServer) GetTask(id int64, out *model.Task) error {
	task, err := s.svc.GetTask(context.Background(), id)
	*out = task
	return err
}

func (s *RPCServer) ListTasks(includeCompleted bool, out *[]model.Task) error {
	tasks, err := s.svc.ListTasks(context.Background(), includeCompleted)
	*out = tasks
	return err
}

func (s *RPCServer) ListTasksFiltered(filter store.ListFilter, out *[]model.Task) error {
	tasks, err := s.svc.ListTasksFiltered(context.Background(), filter)
	*out = tasks
	return err
}

func (s *RPCServer) UpdateTask(in store.UpdateInput, out *model.Task) error {
	task, err := s.svc.UpdateTask(context.Background(), in)
	*out = task
	if err == nil {
		s.notify("gtask", "Updated: " + task.Title)
	}
	return err
}

func (s *RPCServer) DeleteTask(id int64, out *struct{}) error {
	return s.svc.DeleteTask(context.Background(), id)
}

func (s *RPCServer) Sync(dummy int, out *string) error {
	msg, err := s.svc.Sync(context.Background())
	*out = msg
	return err
}
