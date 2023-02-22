package funcframework

import (
	"context"
	"fmt"
	"io"
)

// startRuntimeAPILoop will return an error if handling a particular invoke resulted in a non-recoverable error
func startRuntimeAPILoop(api string, h handlerFunc) error {
	fmt.Printf("api: %s \n", api)
	client := newRuntimeAPIClient(api)
	for {
		invoke, err := client.next()
		fmt.Printf("get invocation %v \n", invoke)
		if err != nil {
			fmt.Printf("fetch invocation error:%v \n", err)
			return err
		}
		if err = handleInvoke(invoke, h); err != nil {
			return err
		}
	}
}

// handleInvoke returns an error if the function panics, or some other non-recoverable error occurred
func handleInvoke(invoke *invoke, handler handlerFunc) error {
	// call the handler, marshal any returned error
	// fmt.Fprintln("payload: %s", invoke.payload)
	response, invokeErr := callBytesHandlerFunc(context.Background(), invoke.payload, handler)
	if invokeErr != nil {
		return nil
	}
	// if the response needs to be closed (ex: net.Conn, os.File), ensure it's closed before the next invoke to prevent a resource leak
	if response, ok := response.(io.Closer); ok {
		defer response.Close()
	}

	// if the response defines a content-type, plumb it through
	contentType := contentTypeBytes
	type ContentType interface{ ContentType() string }
	if response, ok := response.(ContentType); ok {
		contentType = response.ContentType()
	}

	if err := invoke.success(response, contentType); err != nil {
		return fmt.Errorf("unexpected error occurred when sending the function functionResponse to the API: %v", err)
	}

	return nil
}

func callBytesHandlerFunc(ctx context.Context, payload []byte, handler handlerFunc) (response io.Reader, err error) {
	res, err := handler(ctx, payload)
	if err != nil {
		return nil, err
	}
	return res, nil
}
