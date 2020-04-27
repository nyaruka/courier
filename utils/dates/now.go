package dates

import (
	"time"
)

// Now returns the time now.. according to the current source of now
func Now() time.Time {
	return currentNowSource.Now()
}

// NowSource is something that can provide a now result
type NowSource interface {
	Now() time.Time
}

// defaultNowSource returns now as the current system time
type defaultNowSource struct{}

func (s defaultNowSource) Now() time.Time {
	return time.Now()
}

// DefaultNowSource is the default time source
var DefaultNowSource NowSource = defaultNowSource{}
var currentNowSource = DefaultNowSource

// SetNowSource sets the time source used by Now()
func SetNowSource(source NowSource) {
	currentNowSource = source
}

// a source which returns a fixed time
type fixedNowSource struct {
	now time.Time
}

func (s *fixedNowSource) Now() time.Time {
	return s.now
}

// NewFixedNowSource creates a new fixed time now source
func NewFixedNowSource(now time.Time) NowSource {
	return &fixedNowSource{now: now}
}

// a now source which returns a sequence of times 1 second after each other
type sequentialNowSource struct {
	current time.Time
}

func (s *sequentialNowSource) Now() time.Time {
	now := s.current
	s.current = s.current.Add(time.Second * 1)
	return now
}

// NewSequentialNowSource creates a new sequential time source
func NewSequentialNowSource(start time.Time) NowSource {
	return &sequentialNowSource{current: start}
}
