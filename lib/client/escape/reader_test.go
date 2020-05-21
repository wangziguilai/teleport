package escape

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type ReaderSuite struct {
}

var _ = check.Suite(&ReaderSuite{})

type readerTestCase struct {
	inChunks [][]byte
	inErr    error

	wantReadErr       error
	wantDisconnectErr error
	wantOut           string
	wantHelp          string
}

func (*ReaderSuite) runCase(c *check.C, t readerTestCase) {
	in := &mockReader{chunks: t.inChunks, finalErr: t.inErr}
	helpOut := new(bytes.Buffer)
	out := new(bytes.Buffer)
	var disconnectErr error

	r := NewReader(in, helpOut, func(err error) {
		disconnectErr = err
	})

	_, err := io.Copy(out, r)
	c.Assert(err, check.Equals, t.wantReadErr)
	c.Assert(disconnectErr, check.Equals, t.wantDisconnectErr)
	c.Assert(out.String(), check.Equals, t.wantOut)
	c.Assert(helpOut.String(), check.Equals, t.wantHelp)
}

func (s *ReaderSuite) TestNormalReads(c *check.C) {
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte("world"),
		},
		wantOut: "helloworld",
	})
}

func (s *ReaderSuite) TestReadError(c *check.C) {
	customErr := errors.New("oh no")

	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte("world"),
		},
		inErr:             customErr,
		wantOut:           "helloworld",
		wantReadErr:       customErr,
		wantDisconnectErr: customErr,
	})
}

func (s *ReaderSuite) TestEscapeHelp(c *check.C) {
	c.Log("single help sequence between reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeHelp),
			[]byte("world"),
		},
		wantOut:  "hello\r~?world",
		wantHelp: helpText,
	})

	c.Log("single help sequence before any data")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte(escapeHelp),
			[]byte("hello"),
			[]byte("world"),
		},
		wantOut:  "\r~?helloworld",
		wantHelp: helpText,
	})

	c.Log("repeated help sequences")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeHelp),
			[]byte("world"),
			[]byte(escapeHelp),
		},
		wantOut:  "hello\r~?world\r~?",
		wantHelp: helpText + helpText,
	})

	c.Log("help sequence split across two reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeHelp[:1]),
			[]byte(escapeHelp[1:]),
			[]byte("world"),
		},
		wantOut:  "hello\r~?world",
		wantHelp: helpText,
	})

	c.Log("help sequence split across three reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte{escapeHelp[0]},
			[]byte{escapeHelp[1]},
			[]byte{escapeHelp[2]},
			[]byte("world"),
		},
		wantOut:  "hello\r~?world",
		wantHelp: helpText,
	})

	c.Log("incomplete help sequence")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeHelp[:2]),
			[]byte("world"),
		},
		wantOut: "hello\r~world",
	})
}

func (s *ReaderSuite) TestEscapeDisconnect(c *check.C) {
	c.Log("single disconnect sequence between reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeDisconnect),
			[]byte("world"),
		},
		wantOut:           "hello",
		wantReadErr:       ErrDisconnect,
		wantDisconnectErr: ErrDisconnect,
	})

	c.Log("disconnect sequence before any data")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte(escapeDisconnect),
			[]byte("hello"),
			[]byte("world"),
		},
		wantReadErr:       ErrDisconnect,
		wantDisconnectErr: ErrDisconnect,
	})

	c.Log("disconnect sequence split across two reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeDisconnect[:1]),
			[]byte(escapeDisconnect[1:]),
			[]byte("world"),
		},
		wantOut:           "hello\r",
		wantReadErr:       ErrDisconnect,
		wantDisconnectErr: ErrDisconnect,
	})

	c.Log("disconnect sequence split across three reads")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte{escapeDisconnect[0]},
			[]byte{escapeDisconnect[1]},
			[]byte{escapeDisconnect[2]},
			[]byte("world"),
		},
		wantOut:           "hello\r~",
		wantReadErr:       ErrDisconnect,
		wantDisconnectErr: ErrDisconnect,
	})

	c.Log("incomplete disconnect sequence")
	s.runCase(c, readerTestCase{
		inChunks: [][]byte{
			[]byte("hello"),
			[]byte(escapeDisconnect[:2]),
			[]byte("world"),
		},
		wantOut: "hello\r~world",
	})
}

func (*ReaderSuite) TestBufferOverflow(c *check.C) {
	in := &mockReader{chunks: [][]byte{make([]byte, 101)}}
	helpOut := new(bytes.Buffer)
	out := new(bytes.Buffer)
	var disconnectErr error

	r := newUnstartedReader(in, helpOut, func(err error) {
		disconnectErr = err
	})
	r.bufferLimit = 100
	go r.runReads()

	_, err := io.Copy(out, r)
	c.Assert(err, check.Equals, ErrTooMuchBufferedData)
	c.Assert(disconnectErr, check.Equals, ErrTooMuchBufferedData)
	c.Assert(out.Len(), check.Equals, 0)
	c.Assert(helpOut.Len(), check.Equals, 0)
}

type mockReader struct {
	chunks   [][]byte
	finalErr error
}

func (r *mockReader) Read(buf []byte) (int, error) {
	if len(r.chunks) == 0 {
		if r.finalErr != nil {
			return 0, r.finalErr
		}
		return 0, io.EOF
	}

	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]

	copy(buf, chunk)
	return len(chunk), nil

}
