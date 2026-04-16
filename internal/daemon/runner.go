package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os/exec"
	"strings"
	"time"

	"github.com/forechoandlook/gtask/internal/service"
	"github.com/forechoandlook/gtask/internal/store"
)

type Daemon struct {
	svc  service.Service
	host string
	port string
}

func NewDaemon(svc service.Service, host, port string) *Daemon {
	return &Daemon{
		svc:  svc,
		host: host,
		port: port,
	}
}

func (d *Daemon) notify(title, msg string) {
	// 简单的本地通知实现，macOS 使用 osascript
	err := exec.Command("osascript", "-e", fmt.Sprintf(`display notification "%s" with title "%s"`, strings.ReplaceAll(msg, `"`, `\"`), strings.ReplaceAll(title, `"`, `\"`))).Run()
	if err != nil {
		log.Printf("notify err: %v", err)
	}
}

func (d *Daemon) Start() error {
	addr := net.JoinHostPort(d.host, d.port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("gtask daemon listening on %s", addr)
	return d.serveListener(l)
}

func (d *Daemon) serveListener(l net.Listener) error {
	rpcServer := rpc.NewServer()
	srv := &RPCServer{
		svc:    d.svc,
		notify: d.notify,
	}
	err := rpcServer.RegisterName("RPCServer", srv)
	if err != nil {
		return err
	}

	go d.checker()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("accept err: %v", err)
			continue
		}
		go rpcServer.ServeConn(conn)
	}
}

func (d *Daemon) checker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		<-ticker.C
		ctx := context.Background()
		tasks, err := d.svc.ListTasksFiltered(ctx, store.ListFilter{Completed: boolPtr(false)})
		if err != nil {
			log.Printf("checker list tasks err: %v", err)
			continue
		}

		now := time.Now()
		for _, t := range tasks {
			if t.StartAt != nil && isClose(now, *t.StartAt) {
				d.notify("Task Starting Soon", t.Title)
			}
			if t.TargetAt != nil && isClose(now, *t.TargetAt) {
				d.notify("Task Target Approaching", t.Title)
			}
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func isClose(t1, t2 time.Time) bool {
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= 1*time.Minute
}
