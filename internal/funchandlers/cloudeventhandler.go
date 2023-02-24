package funchandlers

import (
	"context"
	"io"

	"encoding/json"

	cloudevents "github.com/cloudevents/sdk-go/v2"
)

type CloudEventHandler struct {
	Fn func(context.Context, cloudevents.Event) error
}

func (h CloudEventHandler) GetHandlerFunc() HandlerFunc {
	return func(ctx context.Context, payload []byte) (io.Reader, error) {
		ev, err := unmarshalPayload(payload)

		if err != nil {
			//fmt.Fprintf(os.Stderr, fmtFunctionError(err))
			return nil, err
		}

		h.Fn(ctx, *ev)

		return nil, nil
	}
}

func unmarshalPayload(payload []byte) (*cloudevents.Event, error) {
	// Validate payload format?
	ev := &cloudevents.Event{}
	err := json.Unmarshal(payload, ev)

	if err != nil {
		return nil, err
	}

	return ev, nil
}
