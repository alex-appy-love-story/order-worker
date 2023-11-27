package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/alex-appy-love-story/db-lib/models/order"
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

func Perform(p StepPayload, ctx TaskContext) (err error) {
	if p.Action == "err" {
		return fmt.Errorf("Test error!!!")
	}

	payloadBuf := new(bytes.Buffer)
	if err := json.NewEncoder(payloadBuf).Encode(&p); err != nil {
		return err
	}

	requestURL := fmt.Sprintf("http://%s", ctx.OrderSvcAddr)
	var res *http.Response
	res, err = http.Post(requestURL, "application/json", payloadBuf)
	if err != nil {
		fmt.Printf("error making http request: %s\n", err)
		return err
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

	log.Printf("%+v\n", ord)

	return PerformNext(p, nextPayload, ctx)
}

func Revert(p StepPayload, ctx TaskContext) error {
	log.Printf("Revert: deleting order ID: %d\n", p.OrderID)

	// Create client.
	client := &http.Client{}
	requestURL := fmt.Sprintf("http://%s/%d", ctx.OrderSvcAddr, p.OrderID)

	// Set up request.
	req, err := http.NewRequest("DELETE", requestURL, nil)
	if err != nil {
		fmt.Printf("error making http request: %s\n", err)
		return err
	}

	// Fetch Request.
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return err
	}
	defer resp.Body.Close()

	// All ok.
	return nil
}
