package test

import (
	"sync"

	"github.com/go-chi/chi"
	"github.com/nyaruka/courier"
)

type MockServer struct {
	backend courier.Backend
	config  *courier.Config

	stopChan chan bool
	stopped  bool
}

func NewMockServer(config *courier.Config, backend courier.Backend) courier.Server {
	return &MockServer{
		backend:  backend,
		config:   config,
		stopChan: make(chan bool),
	}
}

func (ms *MockServer) Config() *courier.Config {
	return ms.config
}

func (ms *MockServer) AddHandlerRoute(handler courier.ChannelHandler, method string, action string, logType courier.ChannelLogType, handlerFunc courier.ChannelHandleFunc) {

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
