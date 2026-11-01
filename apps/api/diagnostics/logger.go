package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrCloseTimeout = errors.New("diagnostic shutdown timed out")
var ErrSinkUnavailable = errors.New("diagnostic persistence unavailable")

// Sink is called by one worker only. It must not call back into Logger. Errors
// never propagate raw path/error text to callers or recursively invoke logging.
type Sink interface {
	WriteEvent([]byte) error
	Flush() error
	Sync() error
	Close() error
}

// Counters are cumulative known losses: queue rejection and failed writes.
// Flush failures can lose previously buffered records in unknown quantities;
// PossibleBufferedLoss records that uncertainty separately.
type Counters struct {
	Debug    uint64 `json:"DEBUG"`
	Info     uint64 `json:"INFO"`
	Warning  uint64 `json:"WARNING"`
	Error    uint64 `json:"ERROR"`
	Critical uint64 `json:"CRITICAL"`
}

func (c *Counters) add(level Level) {
	switch level {
	case Debug:
		c.Debug++
	case Info:
		c.Info++
	case Warning:
		c.Warning++
	case Error:
		c.Error++
	case Critical:
		c.Critical++
	}
}

type queuedEvent struct {
	encodedEvent
	level Level
}

type Logger struct {
	mu                   sync.Mutex
	config               Config
	identity             Identity
	debugUntil           time.Time
	runID                string
	sequence             uint64
	queue                []queuedEvent
	queueBytes           int
	recent               []encodedEvent
	recentBytes          int
	dropped              Counters
	reported             Counters
	closed               bool
	unavailable          bool
	possibleBufferedLoss bool
	closeErr             error
	wake                 chan struct{}
	stop                 chan struct{}
	done                 chan struct{}
	sink                 Sink
}

// New starts the worker. A nil sink enables bounded memory-only diagnostics.
// No filesystem operation is performed on producer goroutines.
func New(config Config, identity Identity, sink Sink) (*Logger, error) {
	config, fallback := ResolveConfig(config)
	runID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.New("could not initialize diagnostic execution identity")
	}
	l := &Logger{config: config, identity: safeIdentity(identity), runID: runID.String(),
		sink: sink, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	if config.MinimumLevel == Debug {
		l.debugUntil = time.Now().Add(DebugDuration)
	}
	go l.run()
	if fallback {
		l.Emit(Input{Event: ConfigFallback})
	}
	return l, nil
}

func (l *Logger) RunID() string { return l.runID }

func (l *Logger) minimumLevel() Level {
	if l.config.MinimumLevel == Debug && time.Now().After(l.debugUntil) {
		return Info
	}
	return l.config.MinimumLevel
}

// Emit returns true when admitted to the queue, not when durable. A rejected
// queue admission can still be present in Recent. Unknown IDs fail closed.
func (l *Logger) Emit(input Input) bool {
	d, ok := Lookup(input.Event)
	if !ok {
		return false
	}
	level, _ := d.Level.SlogLevel()
	minimum, _ := l.minimumLevel().SlogLevel()
	if level < minimum {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.sequence++
	e, ok := encode(input, l.identity, l.runID, l.sequence, time.Now(), nil)
	if !ok {
		l.dropped.add(d.Level)
		return false
	}
	l.remember(e)
	countLimit, byteLimit := QueueEvents, QueueBytes
	if !e.priority {
		countLimit -= PriorityReserveEvents
		byteLimit -= PriorityReserveEvents * MaxEventBytes
	}
	if len(l.queue) >= countLimit || l.queueBytes+len(e.data) > byteLimit {
		l.dropped.add(d.Level)
		return false
	}
	l.queue = append(l.queue, queuedEvent{e, d.Level})
	l.queueBytes += len(e.data)
	select {
	case l.wake <- struct{}{}:
	default:
	}
	return true
}

func (l *Logger) remember(e encodedEvent) {
	for len(l.recent) > 0 && (len(l.recent) >= RecentEvents || l.recentBytes+len(e.data) > RecentBytes) {
		l.recentBytes -= len(l.recent[0].data)
		l.recent[0] = encodedEvent{}
		l.recent = l.recent[1:]
	}
	l.recent = append(l.recent, e)
	l.recentBytes += len(e.data)
}

// Recent returns owned copies, in admission order, already sanitized.
func (l *Logger) Recent() []json.RawMessage {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]json.RawMessage, len(l.recent))
	for i, e := range l.recent {
		result[i] = append(json.RawMessage(nil), e.data...)
	}
	return result
}

func (l *Logger) Dropped() Counters {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

func (l *Logger) PersistenceAvailable() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sink != nil && !l.unavailable && !l.closed
}

// PossibleBufferedLoss remains true after any I/O error because a buffered sink
// can have accepted earlier records that were not yet durable.
func (l *Logger) PossibleBufferedLoss() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.possibleBufferedLoss
}

func (l *Logger) pop() (queuedEvent, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.queue) == 0 {
		return queuedEvent{}, false
	}
	e := l.queue[0]
	l.queue[0] = queuedEvent{}
	l.queue = l.queue[1:]
	l.queueBytes -= len(e.data)
	return e, true
}

func (l *Logger) persistence(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	failed := err != nil
	if failed {
		l.possibleBufferedLoss = true
	}
	if failed == l.unavailable {
		return
	}
	l.unavailable = failed
	id := PersistenceRestored
	if failed {
		id = PersistenceFailed
	}
	l.sequence++
	e, _ := encode(Input{Event: id}, l.identity, l.runID, l.sequence, time.Now(), nil)
	// Internal health transitions are memory-only to avoid recursive failures.
	l.remember(e)
}

func (l *Logger) reportDrops() {
	l.mu.Lock()
	counts := l.dropped
	if counts == l.reported {
		l.mu.Unlock()
		return
	}
	l.sequence++
	e, _ := encode(Input{Event: EventsDropped}, l.identity, l.runID, l.sequence, time.Now(), &counts)
	l.remember(e)
	l.mu.Unlock()
	if l.sink != nil {
		if err := l.sink.WriteEvent(e.data); err != nil {
			l.persistence(err)
			return
		}
	}
	l.mu.Lock()
	l.reported = counts
	l.mu.Unlock()
}

func (l *Logger) write(e queuedEvent) {
	if l.sink == nil {
		return
	}
	err := l.sink.WriteEvent(e.data)
	if err == nil && e.critical {
		err = l.sink.Sync()
	}
	l.persistence(err)
	if err != nil {
		l.mu.Lock()
		l.dropped.add(e.level)
		l.mu.Unlock()
	}
}

func (l *Logger) run() {
	defer close(l.done)
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.wake:
			// Bounded batches let flush/shutdown progress under sustained load.
			for i := 0; i < 128; i++ {
				e, ok := l.pop()
				if !ok {
					break
				}
				l.write(e)
			}
			l.mu.Lock()
			pending := len(l.queue) > 0
			l.mu.Unlock()
			if pending {
				select {
				case l.wake <- struct{}{}:
				default:
				}
			}
		case <-ticker.C:
			l.reportDrops()
			if l.sink != nil {
				l.persistence(l.sink.Flush())
			}
		case <-l.stop:
			for {
				e, ok := l.pop()
				if !ok {
					break
				}
				l.write(e)
			}
			l.reportDrops()
			if l.sink != nil {
				err := l.sink.Sync()
				closeErr := l.sink.Close()
				if err != nil || closeErr != nil {
					l.persistence(ErrSinkUnavailable)
					l.mu.Lock()
					l.closeErr = ErrSinkUnavailable
					l.mu.Unlock()
				}
			}
			return
		}
	}
}

// Close is idempotent; a timed-out worker retains ownership until I/O returns.
// Repeated calls wait for the same worker, never spawn additional workers.
func (l *Logger) Close(ctx context.Context) error {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.stop)
	}
	l.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, ShutdownWait)
	defer cancel()
	select {
	case <-l.done:
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.closeErr
	case <-ctx.Done():
		return ErrCloseTimeout
	}
}
