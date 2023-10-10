package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// FlusherFunc defines our interface for flushers, they are handed a filename and byte blob and are expected
// to try to flush that to the db, returning an error if the db is still down
type FlusherFunc func(filename string, contents []byte) error

// RegisterFlusher creates a new walker which we will use to flush files from the passed in directory
func RegisterFlusher(directory string, flusherFunc FlusherFunc) {
	registeredFlushers = append(registeredFlushers, &flusherRegistration{directory, flusherFunc})
}

// WriteToSpool writes the passed in object to the passed in subdir
func WriteToSpool(spoolDir string, subdir string, contents any) error {
	contentBytes, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return err
	}

	filename := path.Join(spoolDir, subdir, fmt.Sprintf("%d.json", time.Now().UnixNano()))
	return os.WriteFile(filename, contentBytes, 0640)
}

// starts our spool flusher, which every 30 seconds tries to write our pending msgs and statuses
func startSpoolFlushers(s Server) {
	// create our actual flushers
	flushers = make([]*flusher, len(registeredFlushers))
	for i, reg := range registeredFlushers {
		flushers[i] = newSpoolFlusher(s, reg.directory, reg.flusher)
	}

	s.WaitGroup().Add(1)

	go func() {
		defer s.WaitGroup().Done()

		log := slog.With("comp", "spool")
		log.Info("spool started", "state", "started")

		// runs until stopped, checking every 30 seconds if there is anything to flush from our spool
		for {
			select {

			// our server is shutting down, exit
			case <-s.StopChan():
				log.Info("spool stopped", "state", "stopped")
				return

			// every 30 seconds we check to see if there are any files to spool
			case <-time.After(30 * time.Second):
				for _, flusher := range flushers {
					filepath.Walk(flusher.directory, flusher.walker)
				}
			}
		}
	}()
}

// EnsureSpoolDirPresent checks that the passed in spool directory is present and writable
func EnsureSpoolDirPresent(spoolDir string, subdir string) (err error) {
	msgsDir := path.Join(spoolDir, subdir)
	if _, err = os.Stat(msgsDir); os.IsNotExist(err) {
		err = os.MkdirAll(msgsDir, 0770)
	}
	return err
}

// creates a new spool flusher
func newSpoolFlusher(s Server, dir string, flusherFunc FlusherFunc) *flusher {
	return &flusher{func(filename string, info os.FileInfo, err error) error {
		if filename == dir {
			return nil
		}

		// we've been stopped, exit
		if s.Stopped() {
			return errors.New("spool flush process stopped")
		}

		// we don't care about subdirectories
		if info.IsDir() {
			return filepath.SkipDir
		}

		// ignore non-json files
		if !strings.HasSuffix(filename, ".json") {
			return nil
		}

		log := slog.With("comp", "spool", "filename", filename)

		// otherwise, read our msg json
		contents, err := os.ReadFile(filename)
		if err != nil {
			log.Error("reading spool file", "error", err)
			return nil
		}

		err = flusherFunc(filename, contents)
		if err != nil {
			log.Error("flushing spool file", "error", err)
			return err
		}
		log.Info("flushed")

		// we flushed, remove our file if it is still present
		if _, e := os.Stat(filename); e == nil {
			err = os.Remove(filename)
		}
		return err
	}, dir}
}

// simple struct that represents our walking function and the directory that gets walked
type flusher struct {
	walker    filepath.WalkFunc
	directory string
}

var flushers []*flusher

// simple struct to keep track of who has registered to flush and for what directories
type flusherRegistration struct {
	directory string
	flusher   FlusherFunc
}

var registeredFlushers []*flusherRegistration
