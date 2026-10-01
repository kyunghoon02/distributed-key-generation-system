package durable

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

// WAL stores one JSON event per line and syncs every accepted event before
// the participant acknowledges it. A trailing partial line is discarded on
// reopen; malformed complete records fail recovery.
type WAL struct {
	mu     sync.Mutex
	file   *os.File
	failed bool
}

func Open(path string) (*WAL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	return &WAL{file: file}, nil
}

func (w *WAL) ReadAll() ([]protocol.Event, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek journal: %w", err)
	}
	reader := bufio.NewReader(w.file)
	var events []protocol.Event
	var completeBytes int64
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				if err := w.file.Truncate(completeBytes); err != nil {
					return nil, fmt.Errorf("discard incomplete journal tail: %w", err)
				}
				if err := w.file.Sync(); err != nil {
					return nil, fmt.Errorf("sync repaired journal: %w", err)
				}
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read journal: %w", err)
		}
		var event protocol.Event
		if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
			return nil, fmt.Errorf("decode journal record %d: %w", len(events)+1, err)
		}
		events = append(events, event)
		completeBytes += int64(len(line))
	}
	if _, err := w.file.Seek(0, io.SeekEnd); err != nil {
		return nil, fmt.Errorf("seek journal end: %w", err)
	}
	return events, nil
}

func (w *WAL) Append(event protocol.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failed {
		return errors.New("journal is unavailable after a write failure")
	}
	record, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode journal event: %w", err)
	}
	record = append(record, '\n')
	if _, err := w.file.Seek(0, io.SeekEnd); err != nil {
		w.failed = true
		return fmt.Errorf("seek journal end: %w", err)
	}
	n, err := w.file.Write(record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.failed = true
		return fmt.Errorf("write journal: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		w.failed = true
		return fmt.Errorf("sync journal: %w", err)
	}
	return nil
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
