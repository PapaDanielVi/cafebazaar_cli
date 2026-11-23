package httpclient

import (
	"context"
	"io"
)

// ProgressFunc receives the number of bytes transferred and the total expected bytes.
type ProgressFunc func(ctx context.Context, sent, total int64)

// progressReader wraps an io.Reader and reports read progress through a callback.
type progressReader struct {
	ctx   context.Context
	r     io.Reader
	total int64
	sent  int64
	onPct ProgressFunc
}

// newProgressReader creates a reader wrapper that invokes fn as bytes are consumed.
func newProgressReader(ctx context.Context, r io.Reader, total int64, fn ProgressFunc) *progressReader {
	return &progressReader{
		ctx:   ctx,
		r:     r,
		total: total,
		onPct: fn,
	}
}

// Read delegates to the underlying reader and updates the transfer counter.
func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.sent += int64(n)
		if p.onPct != nil {
			p.onPct(p.ctx, p.sent, p.total)
		}
	}
	return n, err
}
