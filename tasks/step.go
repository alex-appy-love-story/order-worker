package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alex-appy-love-story/db-lib/models/order"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

//----------------------------------------------
// Task payload.
//---------------------------------------------
type StepPayload struct {
	SagaPayload

	// Define members here...
	order.OrderInfo

	OrderID  uint   `json:"order_id,omitempty" `
	Username string `json:"username"`
}

func Perform(p StepPayload, taskCtx *TaskContext) (err error) {

	taskCtx.Span.SetAttributes(
		attribute.String("username", p.Username),
		attribute.Int("token_id", int(p.TokenID)),
		attribute.Int("amount", int(p.Amount)),
	)
	taskCtx.Span.AddEvent("Creating order")

	payloadBuf := new(bytes.Buffer)
	if err := json.NewEncoder(payloadBuf).Encode(&p); err != nil {
		return fmt.Errorf("Failed to encode payload")
	}

	taskCtx.Span.AddEvent("Performing an order create request")
	requestURL := fmt.Sprintf("http://%s", taskCtx.OrderSvcAddr)
	var res *http.Response
	res, err = http.Post(requestURL, "application/json", payloadBuf)
	if err != nil {
		return fmt.Errorf("Failed to make order creation request")
	}
	ord := &order.Order{}
	err = json.NewDecoder(res.Body).Decode(&ord)

	p.OrderID = ord.ID

	fmt.Printf("client: got response!\n")
	fmt.Printf("client: status code: %d\n", res.StatusCode)

	nextPayload := map[string]interface{}{
		"token_id": ord.TokenID,
		"user_id":  ord.UserID,
		"username": p.Username,
		"amount":   ord.Amount,
		"order_id": ord.ID,
	}

	taskCtx.Span.AddEvent("Order created", trace.WithAttributes(attribute.Int("order_id", int(p.OrderID))))
	taskCtx.Span.SetAttributes(attribute.Int("order_id", int(p.OrderID)))

    //NOTE(Alex): Same not as Appy but for default response 
	// Immediately send back default response if CB is open
    fmt.Println("Get CB state before: ", taskCtx.CircuitBreaker.State())
	if taskCtx.CircuitBreaker.IsState("open") {
		err = fmt.Errorf("Default response")
		taskCtx.TaskFailed(err)
		errOrder := SetOrderStatus(taskCtx.OrderSvcAddr, ord.ID, order.DEFAULT_RESPONSE)
		if errOrder != nil {
			return fmt.Errorf("Failed to set order status")
		}
		return err
	}

	// NOTE(Appy): We can only force fail after creating the order since we
	//             need the order ID.
	if p.FailTrigger == taskCtx.ServerQueue {
		err = fmt.Errorf("Forced to fail")
		taskCtx.TaskFailed(err)
		errOrder := SetOrderStatus(taskCtx.OrderSvcAddr, ord.ID, order.FORCED_FAIL)
		if errOrder != nil {
			return fmt.Errorf("Failed to set order status")
		}
		return err
	}

	return PerformNext(p, nextPayload, taskCtx)
}

func Revert(p StepPayload, taskCtx *TaskContext) error {
	taskCtx.Span.AddEvent("Reverting order")

	// NOTE(Appy): We don't want to delete the actual record (for testing purposes!)

	// taskCtx.Span.SetAttributes(attribute.Int("order_id", int(p.OrderID)))

	// err := SetOrderStatus(taskCtx.OrderSvcAddr, p.OrderID, order.FAIL)
	// if err != nil {
	// 	return fmt.Errorf("failed to set status")
	// }

	// // Create client.
	// client := &http.Client{}
	// requestURL := fmt.Sprintf("http://%s/%d", taskCtx.OrderSvcAddr, p.OrderID)

	// taskCtx.Span.AddEvent("Creating a delete request")
	// // Set up request.
	// req, err := http.NewRequest("DELETE", requestURL, nil)
	// if err != nil {
	// 	return fmt.Errorf("Failed to create order deletion request")
	// }

	// taskCtx.Span.AddEvent("Performing a delete request")
	// // Fetch Request.
	// resp, err := client.Do(req)
	// if err != nil {
	// 	return fmt.Errorf("Failed to perform order deletion request")
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("Failed to delete order ID %d", p.OrderID)
	// }

	taskCtx.Span.AddEvent("Successfully reverted order creation")
	return nil
}
