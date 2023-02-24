package funchandlers

import (
	"context"
	"io"
)

type HandlerFunc func(context.Context, []byte) (io.Reader, error)

type Handler interface {
	GetHandlerFunc() HandlerFunc
}
