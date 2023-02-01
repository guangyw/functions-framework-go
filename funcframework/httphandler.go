package funcframework

import (
	"bytes"
	"context"
	"net/http"
)

// For backward compatibility.
// Adapt HttpResponseWriter to handlerFunc

type httpHandler struct {
	fn func(http.ResponseWriter, *http.Request)
	//Invoke(ctx context.Context, payload []byte) ([]byte, error)
}

func (h httpHandler) Invoke(ctx context.Context, payload []byte) ([]byte, error) {

	req, err := http.NewRequestWithContext(ctx, "get", "", bytes.NewReader(payload))

	if err != nil {
		return nil, err
	}

	writer := newHttpWriter(ctx)

	h.fn(&writer, req)

	return nil, nil
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

//func adaptHTTPFunction(fn func(http.ResponseWriter, *http.Request)) *Handler {
//
//	if
//
//	return &httpHandler{}
//	//	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	//		if os.Getenv("K_SERVICE") != "" {
//	//			// Force flush of logs after every function trigger when running on GCF.
//	//			defer fmt.Println()
//	//			defer fmt.Fprintln(os.Stderr)
//	//		}
//	//		defer recoverPanic(w, "user function execution")
//	//		fn(w, r)
//	//	}), nil
//}
//
