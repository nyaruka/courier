package test

import (
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/runtime"
	"github.com/nyaruka/courier/utils/clogs"
)

type MockServer struct {
	backend courier.Backend
	rt      *runtime.Runtime

	stopChan chan bool
	stopped  bool
}

func NewMockServer(rt *runtime.Runtime, backend courier.Backend) courier.Server {
	return &MockServer{
		backend:  backend,
		rt:       rt,
		stopChan: make(chan bool),
	}
}

func (ms *MockServer) Runtime() *runtime.Runtime {
	return ms.rt
}

func (ms *MockServer) AddHandlerRoute(handler courier.ChannelHandler, method string, action string, logType clogs.LogType, handlerFunc courier.ChannelHandleFunc) {

}
func (ms *MockServer) GetHandler(courier.Channel) courier.ChannelHandler {
	return nil
}

func (ms *MockServer) Backend() courier.Backend {
	return ms.backend
}

func (ms *MockServer) WaitGroup() *sync.WaitGroup {
	return nil
}
func (ms *MockServer) StopChan() chan bool {
	return ms.stopChan
}
func (ms *MockServer) Stopped() bool {
	return ms.stopped
}

func (ms *MockServer) Router() chi.Router {
	return nil
}

func (ms *MockServer) Start() error { return nil }
func (ms *MockServer) Stop() error {
	ms.stopped = true
	return nil
}
