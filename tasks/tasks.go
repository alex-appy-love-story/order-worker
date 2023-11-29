package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

//----------------------------------------------
// Task payload.
//---------------------------------------------

const (
	PERFORM = "perform"
	REVERT  = "revert"
)

type SagaPayload struct {
	Action string // "perform" or "revert"
}

var (
	tracer = otel.Tracer(os.Getenv("SERVER_QUEUE"))
)

//---------------------------------------------------------------
// Write a function HandleXXXTask to handle the input task.
// Note that it satisfies the asynq.HandlerFunc interface.
//
// Handler doesn't need to be a function. You can define a type
// that satisfies asynq.Handler interface.
//---------------------------------------------------------------

func HandlePerformStepTask(ctx context.Context, t *asynq.Task) error {
	taskContext := GetTaskContext(ctx)

	// Immediately send back default response if CB is open
	if taskContext.CircuitBreaker.IsState("open") {
		return fmt.Errorf("Default response.")
	}

	var p StepPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	// Error channel. This can either catch context cancellation or if an error occured within the task.
	c := make(chan error, 1)

	_, taskContext.Span = tracer.Start(ctx, fmt.Sprintf("%s.perform", taskContext.ServerQueue))
	defer taskContext.Span.End()

	go func() {
		c <- Perform(p, taskContext)
	}()

	var err error
	select {
	case <-ctx.Done():
		// cancelation signal received, abandon this work.
		err = ctx.Err()
	case res := <-c:
		err = res
	}

	if err != nil {
		taskContext.TaskFailed(err)
	} else {
		taskContext.Span.SetStatus(codes.Ok, "")
	}

	return err
}

func HandleRevertStepTask(ctx context.Context, t *asynq.Task) error {
	var p StepPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	taskContext := GetTaskContext(ctx)

	log.Printf("WHAT: %+v\n", taskContext)

	// Error channel. This can either catch context cancellation or if an error occured within the task.
	c := make(chan error, 1)

	_, taskContext.Span = tracer.Start(ctx, fmt.Sprintf("%s.revert", taskContext.ServerQueue))
	defer taskContext.Span.End()

	go func() {
		c <- Revert(p, taskContext)
	}()

	var err error
	select {
	case <-ctx.Done():
		// cancelation signal received, abandon this work.
		err = ctx.Err()
	case res := <-c:
		err = res
	}

	taskContext.AddSpanStateEvent()

	if err != nil {
		taskContext.TaskFailed(err)
	} else {
		taskContext.Span.SetStatus(codes.Ok, "")
	}

	return err
}
