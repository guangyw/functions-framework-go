// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package funcframework is a Functions Framework implementation for Go. It allows you to register
// HTTP and event functions, then start an HTTP server serving those functions.
package funcframework

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
	"strings"

	frameworkclient "github.com/GoogleCloudPlatform/functions-framework-go/internal/client"
	"github.com/GoogleCloudPlatform/functions-framework-go/internal/funchandlers"
	"github.com/GoogleCloudPlatform/functions-framework-go/internal/registry"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

const (
	functionStatusHeader     = "X-Google-Status"
	crashStatus              = "crash"
	errorStatus              = "error"
	panicMessageTmpl         = "A panic occurred during %s. Please see logs for more details."
	fnErrorMessageStderrTmpl = "Function error: %v"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// recoverPanic recovers from a panic in a consistent manner. panicSrc should
// describe what was happening when the panic was encountered, for example
// "user function execution". w is an http.ResponseWriter to write a generic
// response body to that does not expose the details of the panic; w can be
// nil to skip this.
func recoverPanic(w http.ResponseWriter, panicSrc string) {
	if r := recover(); r != nil {
		genericMsg := fmt.Sprintf(panicMessageTmpl, panicSrc)
		fmt.Fprintf(os.Stderr, "%s\npanic message: %v\nstack trace: %s", genericMsg, r, debug.Stack())
		if w != nil {
			writeHTTPErrorResponse(w, http.StatusInternalServerError, crashStatus, genericMsg)
		}
	}
}

// RegisterHTTPFunction registers fn as an HTTP function.
// Maintained for backward compatibility. Please use RegisterHTTPFunctionContext instead.
func RegisterHTTPFunction(path string, fn interface{}) {
	defer recoverPanic(nil, "function registration")

	fnHTTP, ok := fn.(func(http.ResponseWriter, *http.Request))
	if !ok {
		panic("expected function to have signature func(http.ResponseWriter, *http.Request)")
	}

	ctx := context.Background()
	if err := RegisterHTTPFunctionContext(ctx, path, fnHTTP); err != nil {
		panic(fmt.Sprintf("unexpected error in RegisterEventFunctionContext: %v", err))
	}
}

// RegisterEventFunction registers fn as an event function.
// Maintained for backward compatibility. Please use RegisterEventFunctionContext instead.
func RegisterEventFunction(path string, fn interface{}) {
	ctx := context.Background()
	defer recoverPanic(nil, "function registration")
	if err := RegisterEventFunctionContext(ctx, path, fn); err != nil {
		panic(fmt.Sprintf("unexpected error in RegisterEventFunctionContext: %v", err))
	}
}

// RegisterHTTPFunctionContext registers fn as an HTTP function.
func RegisterHTTPFunctionContext(ctx context.Context, path string, fn func(http.ResponseWriter, *http.Request)) error {
	return registry.Default().RegisterHTTP(fn, registry.WithPath(path))
}

// RegisterEventFunctionContext registers fn as an event function. The function must have two arguments, a
// context.Context and a struct type depending on the event, and return an error. If fn has the
// wrong signature, RegisterEventFunction returns an error.
func RegisterEventFunctionContext(ctx context.Context, path string, fn interface{}) error {
	return registry.Default().RegisterEvent(fn, registry.WithPath(path))
}

// RegisterCloudEventFunctionContext registers fn as an cloudevent function.
func RegisterCloudEventFunctionContext(ctx context.Context, path string, fn func(context.Context, cloudevents.Event) error) error {
	return registry.Default().RegisterCloudEvent(fn, registry.WithPath(path))
}

func Start() {
	// If FUNCTION_TARGET is set, only serve this target function at path "/".
	// If not set, serve all functions at the registered paths.
	if target := os.Getenv("FUNCTION_TARGET"); len(target) > 0 {
		var targetFn *registry.RegisteredFunction

		fn, ok := registry.Default().GetRegisteredFunction(target)
		if ok {
			targetFn = fn
		} else if lastFnWithoutName := registry.Default().GetLastFunctionWithoutName(); lastFnWithoutName != nil {
			// If no function was found with the target name, assume the last function that's not registered declaratively
			// should be served at '/'.
			targetFn = lastFnWithoutName
		} else {
			return
		}

		h := targetFn.GetHandler()

		apiAddr := os.Getenv("AWS_LAMBDA_RUNTIME_API")
		if apiAddr != "" {
			startRequestLoop(apiAddr, h.GetHandlerFunc())
		} else {
			startRequestLoop("127.0.0.1:9001", h.GetHandlerFunc())
		}
	}

	//fns := registry.Default().GetAllFunctions()
	//for _, fn := range fns {
	//	h, err := wrapFunction(fn)
	//}
}

func startRequestLoop(api string, h funchandlers.HandlerFunc) error {
	fmt.Printf("api: %s \n", api)
	client := frameworkclient.New(api)
	for {
		invoke, err := client.Next()
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
func handleInvoke(invoke *frameworkclient.Invoke, handler funchandlers.HandlerFunc) error {
	// call the handler, marshal any returned error
	// fmt.Fprintln("payload: %s", invoke.payload)
	response, invokeErr := handler(context.Background(), invoke.Payload)
	if invokeErr != nil {
		return nil
	}
	// if the response needs to be closed (ex: net.Conn, os.File), ensure it's closed before the next invoke to prevent a resource leak
	if response, ok := response.(io.Closer); ok {
		defer response.Close()
	}

	// if the response defines a content-type, plumb it through
	contentType := "application/octet-stream"
	type ContentType interface{ ContentType() string }
	if response, ok := response.(ContentType); ok {
		contentType = response.ContentType()
	}

	if err := invoke.Success(response, contentType); err != nil {
		return fmt.Errorf("unexpected error occurred when sending the function functionResponse to the API: %v", err)
	}

	return nil
}

func runUserFunction(w http.ResponseWriter, r *http.Request, data []byte, fn interface{}) {
	runUserFunctionWithContext(r.Context(), w, r, data, fn)
}

func runUserFunctionWithContext(ctx context.Context, w http.ResponseWriter, r *http.Request, data []byte, fn interface{}) {
	argVal := reflect.New(reflect.TypeOf(fn).In(1))
	if err := json.Unmarshal(data, argVal.Interface()); err != nil {
		writeHTTPErrorResponse(w, http.StatusBadRequest, crashStatus, fmt.Sprintf("Error: %s, while converting event data: %s", err.Error(), string(data)))
		return
	}

	defer recoverPanic(w, "user function execution")
	userFunErr := reflect.ValueOf(fn).Call([]reflect.Value{
		reflect.ValueOf(ctx),
		argVal.Elem(),
	})
	if userFunErr[0].Interface() != nil {
		writeHTTPErrorResponse(w, http.StatusInternalServerError, errorStatus, fmtFunctionError(userFunErr[0].Interface()))
		return
	}
}

func fmtFunctionError(err interface{}) string {
	formatted := fmt.Sprintf(fnErrorMessageStderrTmpl, err)
	if !strings.HasSuffix(formatted, "\n") {
		formatted += "\n"
	}

	return formatted
}

func writeHTTPErrorResponse(w http.ResponseWriter, statusCode int, status, msg string) {
	// Ensure logs end with a newline otherwise they are grouped incorrectly in SD.
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprint(os.Stderr, msg)

	// Flush stdout and stderr when running on GCF. This must be done before writing
	// the HTTP response in order for all logs to appear in GCF.
	if os.Getenv("K_SERVICE") != "" {
		fmt.Println()
		fmt.Fprintln(os.Stderr)
	}

	w.Header().Set(functionStatusHeader, status)
	w.WriteHeader(statusCode)
	fmt.Fprint(w, msg)
}
