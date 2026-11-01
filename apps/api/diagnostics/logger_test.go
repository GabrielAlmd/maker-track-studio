package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type memorySink struct{ bytes.Buffer }

func (s *memorySink) WriteEvent(data []byte) error { _, err := s.Write(data); return err }
func (*memorySink) Flush() error                   { return nil }
func (*memorySink) Sync() error                    { return nil }
func (*memorySink) Close() error                   { return nil }

func newTestLogger(t *testing.T, sink Sink) *Logger {
	t.Helper()
	l, err := New(DefaultConfig(), Identity{Version: "1.2.3", BuildID: "build-123"}, sink)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close(context.Background()) })
	return l
}

type forbiddenValue struct{}

func (forbiddenValue) LogValue() slog.Value { panic("opaque value evaluated") }
func (forbiddenValue) String() string       { panic("opaque value formatted") }

func TestSanitizationBeforeEveryDestination(t *testing.T) {
	sink := &memorySink{}
	l, err := New(DefaultConfig(), Identity{Version: "secret-token", BuildID: "/home/customer/private"}, sink)
	if err != nil {
		t.Fatal(err)
	}
	l.Emit(Input{Event: BackupExportFailed, RequestID: "Bearer secret-token", OperationID: "/home/customer/private",
		ErrorCode: ErrorCode("Password: secret-token"), Duration: -time.Second})
	s := l.Slog().With("Authorization", forbiddenValue{}, "request_id", "secret-token")
	s.Error(string(BackupExportFailed), "error_code", "secret-token", "error", forbiddenValue{},
		"duration", time.Second, "password", "secret-token")
	s.WithGroup("private").Error(string(BackupExportFailed), "operation_id", "secret-token")
	s.Error("secret-token") // unknown message is not accepted as a catalog ID
	if err := l.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	outputs := append(l.Recent(), json.RawMessage(sink.Bytes()))
	for _, output := range outputs {
		for _, forbidden := range []string{"secret-token", "/home/customer", "Authorization", "password"} {
			if bytes.Contains(output, []byte(forbidden)) {
				t.Fatalf("secret in output: %s", output)
			}
		}
	}
	lines := bytes.Split(bytes.TrimSpace(sink.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("got %d events", len(lines))
	}
	for _, line := range lines {
		var event wireEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event.AppVersion != "unknown" || event.BuildID != "unknown" || event.Level != Error || event.DurationMS < 0 {
			t.Fatalf("unsafe event: %+v", event)
		}
	}
}

func TestRecentSnapshotOwnershipAndLimits(t *testing.T) {
	l := newTestLogger(t, nil)
	for i := 0; i < 1000; i++ {
		l.Emit(Input{Event: AppReady})
	}
	snapshot := l.Recent()
	if len(snapshot) != RecentEvents {
		t.Fatalf("recent length: %d", len(snapshot))
	}
	snapshot[0][0] = '!'
	if !json.Valid(l.Recent()[0]) {
		t.Fatal("snapshot modified internal buffer")
	}
	var last uint64
	for _, raw := range l.Recent() {
		var event wireEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Sequence <= last {
			t.Fatal("unordered recent events")
		}
		last = event.Sequence
	}
	if l.Emit(Input{Event: RequestCompleted}) {
		t.Fatal("DEBUG admitted at INFO")
	}
}

type blockedSink struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockedSink) WriteEvent([]byte) error {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	return nil
}
func (*blockedSink) Flush() error { return nil }
func (*blockedSink) Sync() error  { return nil }
func (*blockedSink) Close() error { return nil }

func TestQueueReserveAndBoundedShutdown(t *testing.T) {
	sink := &blockedSink{entered: make(chan struct{}), release: make(chan struct{})}
	l := newTestLogger(t, sink)
	defer close(sink.release)
	l.Emit(Input{Event: AppReady})
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	for i := 0; i < QueueEvents-PriorityReserveEvents; i++ {
		if !l.Emit(Input{Event: AppReady}) {
			t.Fatalf("early rejection: %d", i)
		}
	}
	if l.Emit(Input{Event: AppReady}) {
		t.Fatal("low priority used reserve")
	}
	for i := 0; i < PriorityReserveEvents; i++ {
		if !l.Emit(Input{Event: StateUnsafe}) {
			t.Fatal("priority reserve unavailable")
		}
	}
	if l.Emit(Input{Event: StateUnsafe}) {
		t.Fatal("queue grew beyond limit")
	}
	if got := l.Dropped(); got.Info != 1 || got.Critical != 1 {
		t.Fatalf("wrong losses: %+v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Close(ctx); !errors.Is(err, ErrCloseTimeout) {
		t.Fatalf("close: %v", err)
	}
	if l.Emit(Input{Event: AppReady}) {
		t.Fatal("accepted after close")
	}
}

func TestConcurrentProducers(t *testing.T) {
	sink := &memorySink{}
	l := newTestLogger(t, sink)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				l.Emit(Input{Event: AppReady})
			}
		}()
	}
	wg.Wait()
	if err := l.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := map[uint64]bool{}
	var persisted uint64
	for _, line := range strings.Split(strings.TrimSpace(sink.String()), "\n") {
		var event wireEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if seen[event.Sequence] {
			t.Fatal("duplicate sequence")
		}
		seen[event.Sequence] = true
		if event.Event == AppReady {
			persisted++
		}
	}
	if persisted+l.Dropped().Info != 100000 {
		t.Fatal("events lost without accounting")
	}
	if len(l.Recent()) > RecentEvents {
		t.Fatal("unbounded recent buffer")
	}
}

type failedSink struct{}

func (failedSink) WriteEvent([]byte) error { return errors.New("sensitive filesystem path") }
func (failedSink) Flush() error            { return errors.New("sensitive filesystem path") }
func (failedSink) Sync() error             { return errors.New("sensitive filesystem path") }
func (failedSink) Close() error            { return errors.New("sensitive filesystem path") }

func TestSinkFailureIsSanitizedAndDoesNotRecurse(t *testing.T) {
	l := newTestLogger(t, failedSink{})
	for i := 0; i < 5; i++ {
		l.Emit(Input{Event: BackupExportFailed})
	}
	if err := l.Close(context.Background()); !errors.Is(err, ErrSinkUnavailable) {
		t.Fatalf("close: %v", err)
	}
	if l.Dropped().Error != 5 || l.PersistenceAvailable() || !l.PossibleBufferedLoss() {
		t.Fatal("incorrect failed sink status")
	}
	if len(l.Recent()) > 7 {
		t.Fatal("recursive error reporting")
	}
	for _, event := range l.Recent() {
		if bytes.Contains(event, []byte("sensitive")) {
			t.Fatal("raw sink error leaked")
		}
	}
}
