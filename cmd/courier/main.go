package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"

	// load channel handler packages
	_ "github.com/nyaruka/courier/handlers/africastalking"
	_ "github.com/nyaruka/courier/handlers/blackmyna"
	_ "github.com/nyaruka/courier/handlers/kannel"
	_ "github.com/nyaruka/courier/handlers/telegram"
	_ "github.com/nyaruka/courier/handlers/twilio"

	// load available backends
	_ "github.com/nyaruka/courier/backends/rapidpro"
)

func main() {
	m := config.NewWithPath("courier.toml")
	config := &config.Courier{}

	err := m.Load(config)
	if err != nil {
		log.Fatalf("Error loading configuration: %s", err)
	}

	// load our backend
	backend, err := courier.NewBackend(config)
	if err != nil {
		log.Fatalf("Error creating backend: %s", err)
	}

	server := courier.NewServer(config, backend)
	err = server.Start()
	if err != nil {
		log.Fatalf("Error starting server: %s", err)
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	server.Stop()
}
