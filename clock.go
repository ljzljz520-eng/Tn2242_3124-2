package fruitcut

import (
	"fmt"
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type FixedClock struct {
	mu      sync.RWMutex
	current time.Time
}

func NewFixedClock(current time.Time) *FixedClock {
	return &FixedClock{current: current}
}

func (c *FixedClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *FixedClock) Set(current time.Time) {
	c.mu.Lock()
	c.current = current
	c.mu.Unlock()
}

type IDGenerator interface {
	Next(prefix string) string
}

type SequenceIDGenerator struct {
	mu   sync.Mutex
	next int
}

func NewSequenceIDGenerator(start int) *SequenceIDGenerator {
	if start < 1 {
		start = 1
	}
	return &SequenceIDGenerator{next: start}
}

func (g *SequenceIDGenerator) Next(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := fmt.Sprintf("%s-%03d", prefix, g.next)
	g.next++
	return id
}
