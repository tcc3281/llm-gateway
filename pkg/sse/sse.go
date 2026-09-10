package sse

import (
	"bufio"
	"bytes"
	"io"
	"sync"
)

type Event struct {
	Event string
	Data  []byte
}

type Reader struct {
	scanner *bufio.Scanner
	bufPtr  *[]byte
}

const (
	defaultBufferSize = 64 * 1024
	maxBufferSize     = 10 * 1024 * 1024 // 10MB to accommodate large tool call parameters or reasoning chains
)

var (
	eventPrefix = []byte("event:")
	dataPrefix  = []byte("data:")

	scannerBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, defaultBufferSize)
			return &b
		},
	}
)

func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	bufPtr := scannerBufferPool.Get().(*[]byte)
	scanner.Buffer(*bufPtr, maxBufferSize)
	return &Reader{
		scanner: scanner,
		bufPtr:  bufPtr,
	}
}

// Close returns the scanner buffer to the pool.
func (r *Reader) Close() {
	if r.bufPtr != nil {
		scannerBufferPool.Put(r.bufPtr)
		r.bufPtr = nil
	}
}

func (r *Reader) ReadEvent() (*Event, error) {
	var currentEvent Event
	var dataBuffer bytes.Buffer

	for r.scanner.Scan() {
		line := r.scanner.Bytes()
		if len(line) == 0 {
			// Empty line signifies end of an SSE message block
			if dataBuffer.Len() > 0 || currentEvent.Event != "" {
				currentEvent.Data = trimTrailingNewline(dataBuffer.Bytes())
				return &currentEvent, nil
			}
			continue
		}

		if bytes.HasPrefix(line, eventPrefix) {
			currentEvent.Event = string(bytes.TrimSpace(line[len(eventPrefix):]))
		} else if bytes.HasPrefix(line, dataPrefix) {
			dataBuffer.Write(bytes.TrimSpace(line[len(dataPrefix):]))
			dataBuffer.WriteByte('\n')
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	if dataBuffer.Len() > 0 {
		currentEvent.Data = trimTrailingNewline(dataBuffer.Bytes())
		return &currentEvent, nil
	}

	return nil, io.EOF
}

// trimTrailingNewline trims a single trailing '\n' without any slice allocations.
func trimTrailingNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}
