package sse

import (
	"bufio"
	"bytes"
	"io"
)

type Event struct {
	Event string
	Data  []byte
}

type Reader struct {
	scanner *bufio.Scanner
}

const (
	defaultBufferSize = 64 * 1024
	maxBufferSize     = 10 * 1024 * 1024 // 10MB to accommodate large tool call parameters or reasoning chains
)

func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, defaultBufferSize)
	scanner.Buffer(buf, maxBufferSize)
	return &Reader{
		scanner: scanner,
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
				currentEvent.Data = bytes.TrimSuffix(dataBuffer.Bytes(), []byte("\n"))
				return &currentEvent, nil
			}
			continue
		}

		if bytes.HasPrefix(line, []byte("event:")) {
			currentEvent.Event = string(bytes.TrimSpace(line[6:]))
		} else if bytes.HasPrefix(line, []byte("data:")) {
			dataBuffer.Write(bytes.TrimSpace(line[5:]))
			dataBuffer.WriteByte('\n')
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	if dataBuffer.Len() > 0 {
		currentEvent.Data = bytes.TrimSuffix(dataBuffer.Bytes(), []byte("\n"))
		return &currentEvent, nil
	}

	return nil, io.EOF
}
