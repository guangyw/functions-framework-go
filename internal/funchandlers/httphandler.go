package funchandlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// For backward compatibility.
// Adapt HttpResponseWriter to handlerFunc

type HttpHandler struct {
	Fn func(http.ResponseWriter, *http.Request)
	//Invoke(ctx context.Context, payload []byte) ([]byte, error)
}

func (h HttpHandler) GetHandlerFunc() HandlerFunc {
	return func(ctx context.Context, payload []byte) (io.Reader, error) {
		req, err := http.NewRequestWithContext(ctx, "get", "", bytes.NewReader(payload))

		if err != nil {
			return nil, err
		}

		writer := newHttpWriter(ctx)

		h.Fn(&writer, req)

		b, err := io.ReadAll(&writer.Buffer)

		if err != nil {
			return nil, err
		}

		return bytes.NewReader(b), nil
	}
}

type httpWriter struct {
	bytes.Buffer

	header map[string][]string
}

func (w *httpWriter) WriteHeader(statusCode int) {

}

func (w *httpWriter) Header() http.Header {
	return w.header
}

func newHttpWriter(ctx context.Context) httpWriter {
	return httpWriter{
		//TODO: get header from context
		//header: ctx.Value("header")

	}

}
