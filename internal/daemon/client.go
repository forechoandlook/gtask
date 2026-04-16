package daemon

import (
	"context"
	"net/rpc"

	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
)

type RPCClient struct {
	client *rpc.Client
}

func NewRPCClient(network, address string) (service.Service, error) {
	client, err := rpc.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return &RPCClient{client: client}, nil
}

func (c *RPCClient) AddTask(ctx context.Context, in store.AddInput) (model.Task, error) {
	var out model.Task
	err := c.client.Call("RPCServer.AddTask", in, &out)
	return out, err
}

func (c *RPCClient) GetTask(ctx context.Context, id int64) (model.Task, error) {
	var out model.Task
	err := c.client.Call("RPCServer.GetTask", id, &out)
	return out, err
}

func (c *RPCClient) ListTasks(ctx context.Context, includeCompleted bool) ([]model.Task, error) {
	var out []model.Task
	err := c.client.Call("RPCServer.ListTasks", includeCompleted, &out)
	return out, err
}

func (c *RPCClient) ListTasksFiltered(ctx context.Context, filter store.ListFilter) ([]model.Task, error) {
	var out []model.Task
	err := c.client.Call("RPCServer.ListTasksFiltered", filter, &out)
	return out, err
}

func (c *RPCClient) UpdateTask(ctx context.Context, in store.UpdateInput) (model.Task, error) {
	var out model.Task
	err := c.client.Call("RPCServer.UpdateTask", in, &out)
	return out, err
}

func (c *RPCClient) DeleteTask(ctx context.Context, id int64) error {
	var out struct{}
	return c.client.Call("RPCServer.DeleteTask", id, &out)
}

func (c *RPCClient) Sync(ctx context.Context) (string, error) {
	var out string
	err := c.client.Call("RPCServer.Sync", 0, &out)
	return out, err
}
