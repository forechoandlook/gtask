package daemon

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/forechoandlook/gtask/internal/model"
	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
)

type mockService struct {
	service.Service
	added bool
}

func (m *mockService) AddTask(ctx context.Context, in store.AddInput) (model.Task, error) {
	m.added = true
	return model.Task{ID: 1, Title: in.Title}, nil
}

func TestDaemonClient(t *testing.T) {
	svc := &mockService{}
	d := NewDaemon(svc, "localhost", "0") // 0 means dynamic port

	// We need to start daemon but know its port
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		// Just serve this one listener
		d.serveListener(context.Background(), l)
	}()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	clientSvc, err := NewRPCClient("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	task, err := clientSvc.AddTask(context.Background(), store.AddInput{Title: "hello"})
	if err != nil {
		t.Fatal(err)
	}

	if task.Title != "hello" {
		t.Errorf("expected hello, got %s", task.Title)
	}
	if !svc.added {
		t.Errorf("expected mock service to be called")
	}
}
