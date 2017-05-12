package courier

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// starts our spool flusher, which every 30 seconds tries to write our pending msgs and statuses
func startMsgSpoolFlusher(s *server) {
	s.waitGroup.Add(1)
	defer s.waitGroup.Done()

	msgsDir := path.Join(s.config.SpoolDir, "msgs")
	statusesDir := path.Join(s.config.SpoolDir, "statuses")

	msgWalker := s.msgSpoolWalker(msgsDir)
	statusWalker := s.statusSpoolWalker(statusesDir)

	log.Println("[X] Spool: flush process started")

	// runs until stopped, checking every 30 seconds if there is anything to flush from our spool
	for {
		select {

		// our server is shutting down, exit
		case <-s.stopChan:
			log.Println("[X] Spool: flush process stopped")
			return

		// every 30 seconds we check to see if there are any files to spool
		case <-time.After(30 * time.Second):
			filepath.Walk(msgsDir, msgWalker)
			filepath.Walk(statusesDir, statusWalker)
		}
	}

}

// checks that our spool directories are present and writable
func testSpoolDirs(s *server) (err error) {
	msgsDir := path.Join(s.config.SpoolDir, "msgs")
	if _, err = os.Stat(msgsDir); os.IsNotExist(err) {
		err = os.MkdirAll(msgsDir, 0770)
	}
	if err != nil {
		return err
	}

	statusesDir := path.Join(s.config.SpoolDir, "statuses")
	if _, err = os.Stat(statusesDir); os.IsNotExist(err) {
		err = os.MkdirAll(statusesDir, 0770)
	}
	return err
}

// writes the passed in object to the passed in subdir
func writeToSpool(s *server, subdir string, contents interface{}) error {
	contentBytes, err := json.Marshal(contents)
	if err != nil {
		return err
	}

	filename := path.Join(s.config.SpoolDir, subdir, fmt.Sprintf("%d.json", time.Now().UnixNano()))
	return ioutil.WriteFile(filename, contentBytes, 0640)
}

// our interface for flushers, they are handed a filename and byte blob and are expected to try to flush
// that to the db, returning an error if the db is still down
type fileFlusher func(filename string, contents []byte) error

// creates a new spool walker
func (s *server) newSpoolWalker(dir string, flusher fileFlusher) filepath.WalkFunc {
	return func(filename string, info os.FileInfo, err error) error {
		if filename == dir {
			return nil
		}

		// we've been stopped, exit
		if s.stopped {
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

		// otherwise, read our msg json
		contents, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("ERROR reading spool file '%s': %s\n", filename, err)
			return nil
		}

		err = flusher(filename, contents)
		if err != nil {
			log.Printf("ERROR flushing file '%s': %s\n", filename, err)
			return err
		}

		log.Printf("Spool: flushed '%s' to redis", filename)

		// we flushed to redis, remove our file if it is still present
		if _, e := os.Stat(filename); e == nil {
			err = os.Remove(filename)
		}
		return err
	}
}
