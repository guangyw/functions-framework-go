package funcframework

import (
	"context"
	"io"
)

type handlerFunc func(context.Context, []byte) (io.Reader, error)

type Handler interface {
	Invoke(ctx context.Context, payload []byte) ([]byte, error)
}
