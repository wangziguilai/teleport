/*
Copyright 2020 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package escape implements client-side escape character logic.
// This logic mimics OpenSSH: https://man.openbsd.org/ssh#ESCAPE_CHARACTERS.
package escape

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

const (
	readerBufferLimit = 10 * 1 << 10 // 10MB

	// Supported escape sequences.
	escapeHelp       = "\r~?"
	escapeDisconnect = "\r~."
	// Note: on a raw terminal, "\r\n" is needed to move a cursor to the start
	// of next line.
	helpText = "\r\ntsh escape characters:\r\n  ~? - display a list of escape characters\r\n  ~. - disconnect\r\n"
)

var (
	// ErrDisconnect is returned when the user has entered a disconnect
	// sequence, requesting connection to be interrupted.
	ErrDisconnect = errors.New("disconnect escape sequence detected")
	// ErrTooMuchBufferedData is returned when the Reader's internal buffer has
	// filled over 10MB. Either the consumer of Reader can't keep up with the
	// data or it's entirely stuck and not consuming the data.
	ErrTooMuchBufferedData = errors.New("internal buffer has grown too big")
)

// Reader is an io.Reader wrapper that catches OpenSSH-like escape sequences in
// the input stream. See NewReader for more info.
//
// Reader is safe for concurrent use.
type Reader struct {
	inner        io.Reader
	out          io.Writer
	onDisconnect func(error)
	bufferLimit  int

	// cond protects buf and err and also announces to blocked readers that
	// more data is available.
	cond sync.Cond
	buf  []byte
	err  error
}

// NewReader creates a new Reader to catch escape sequences from 'in'.
//
// Two sequences are supported:
// - "~?": prints help text to 'out' listing supported sequences
// - "~.": disconnect stops any future reads from in; after this sequence,
//   callers can still read any unread data up to this sequence from Reader but
//   all future Read calls will return ErrDisconnect; onDisconnect will also be
//   called with ErrDisconnect immediately
//
// NewReader starts consuming 'in' immediately in the background. This allows
// Reader to detect sequences without the caller actively calling Read (such as
// when it's stuck writing out the received data).
//
// Unread data is accumulated in an internal buffer. If this buffer grows to a
// limit (currently 10MB), Reader will stop permanently. onDisconnect will get
// called with ErrTooMuchBufferedData. Read can still be called to consume the
// internal buffer but all future reads after that will return
// ErrTooMuchBufferedData.
//
// If the internal buffer is empty, calls to Read will block until some data is
// available or an error occurs.
func NewReader(in io.Reader, out io.Writer, onDisconnect func(error)) *Reader {
	r := newUnstartedReader(in, out, onDisconnect)
	go r.runReads()
	return r
}

// newUnstartedReader allows unit tests to mutate Reader before runReads
// starts.
func newUnstartedReader(in io.Reader, out io.Writer, onDisconnect func(error)) *Reader {
	return &Reader{
		inner:        in,
		out:          out,
		onDisconnect: onDisconnect,
		bufferLimit:  readerBufferLimit,
		cond:         sync.Cond{L: &sync.Mutex{}},
		// note: no need to pre-allocate buf, it will allocate and grow as
		// needed in runReads via append.
	}
}

func (r *Reader) runReads() {
	// prev contains up to 2 characters from previous Read to catch sequences
	// spanning multiple reads.
	prev := []byte{'\r'}
	// Use a small read buffer. We're reading terminal input, so there will
	// only be a few characters at a time.
	readBuf := make([]byte, 128)
	for {
		n, err := r.inner.Read(readBuf)
		if err != nil {
			r.setErr(err)
			return
		}

		// Check for escape sequences.
		keys := append(prev, readBuf[:n]...)
		switch {
		case bytes.Contains(keys, []byte(escapeHelp)):
			r.printHelp()
		case bytes.Contains(keys, []byte(escapeDisconnect)):
			r.setErr(ErrDisconnect)
			return
		}

		// Reset prev to the last 2 characters read.
		prev = keys
		if len(prev) > 2 {
			prev = prev[len(prev)-2:]
		}

		// Add new data to internal buffer.
		r.cond.L.Lock()
		if len(r.buf)+n > r.bufferLimit {
			// Unlock because setErr will want to lock too.
			r.cond.L.Unlock()
			r.setErr(ErrTooMuchBufferedData)
			return
		}
		r.buf = append(r.buf, readBuf[:n]...)
		// Notify blocked Read calls about new data.
		r.cond.Broadcast()
		r.cond.L.Unlock()
	}
}

// Read fills buf with available data. If no data is available, Read will
// block.
func (r *Reader) Read(buf []byte) (int, error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	// Block until some data was read in runReads.
	for len(r.buf) == 0 && r.err == nil {
		r.cond.Wait()
	}

	// Have some data to return.
	n := len(r.buf)
	if n > len(buf) {
		n = len(buf)
	}
	// Write n available bytes to buf and trim them from r.buf.
	copy(buf, r.buf[:n])
	r.buf = r.buf[n:]

	return n, r.err
}

func (r *Reader) setErr(err error) {
	r.cond.L.Lock()
	r.err = err
	r.cond.Broadcast()
	// Skip EOF, it's a normal clean exit.
	if err != io.EOF {
		r.onDisconnect(err)
	}
	r.cond.L.Unlock()
}

func (r *Reader) printHelp() {
	r.out.Write([]byte(helpText))
}
