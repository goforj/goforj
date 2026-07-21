package managedsession

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	// ErrClosed reports a managed-session operation after the connection closed.
	ErrClosed = errors.New("managed session is closed")
	// ErrEmptyFrame reports a zero-length frame.
	ErrEmptyFrame = errors.New("managed session frame is empty")
	// ErrFrameUnusable reports a stream whose frame boundary can no longer be trusted.
	ErrFrameUnusable = errors.New("managed session frame stream is unusable")
)

// FrameSizeError reports a frame length above the negotiated hard limit.
type FrameSizeError struct {
	Size  uint32
	Limit uint32
}

// Error returns a bounded frame-size diagnostic.
func (errorValue FrameSizeError) Error() string {
	return fmt.Sprintf("managed session frame size %d exceeds limit %d", errorValue.Size, errorValue.Limit)
}

// frameReader reads four-byte big-endian length-prefixed JSON frames.
type frameReader struct {
	reader   io.Reader
	limit    uint32
	unusable bool
}

// readFrame returns one complete JSON payload.
func (reader *frameReader) readFrame() ([]byte, error) {
	if reader.unusable {
		return nil, ErrFrameUnusable
	}
	var header [4]byte
	read, err := io.ReadFull(reader.reader, header[:])
	if err != nil {
		if read > 0 {
			reader.unusable = true
		}
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		reader.unusable = true
		return nil, ErrEmptyFrame
	}
	if size > reader.limit {
		reader.unusable = true
		return nil, FrameSizeError{Size: size, Limit: reader.limit}
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader.reader, payload); err != nil {
		reader.unusable = true
		return nil, err
	}
	if !json.Valid(payload) {
		return nil, errors.New("managed session frame is not valid JSON")
	}
	return payload, nil
}

// frameWriter writes serialized JSON frames and prevents interleaved prefixes.
type frameWriter struct {
	writer   io.Writer
	limit    uint32
	mutex    sync.Mutex
	unusable bool
}

// writeFrame writes one already-encoded JSON value with its length prefix.
func (writer *frameWriter) writeFrame(payload []byte) error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.unusable {
		return ErrFrameUnusable
	}
	if len(payload) == 0 {
		return ErrEmptyFrame
	}
	if uint64(len(payload)) > uint64(writer.limit) {
		return FrameSizeError{Size: writer.limit + 1, Limit: writer.limit}
	}
	if !json.Valid(payload) {
		return errors.New("managed session frame is not valid JSON")
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	if err := writeAll(writer.writer, frame); err != nil {
		writer.unusable = true
		return err
	}
	return nil
}

// writeAll treats a short or stalled write as terminal for the stream.
func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrNoProgress
		}
		payload = payload[written:]
	}
	return nil
}
