package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"focus-ai/config"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/avast/retry-go"
)

func makeRequest(method string, url string, reqParams, respStruct interface{}) (err error) {
	return retry.Do(func() error {
		var reqBody []byte
		if reqParams != nil {
			reqBody, err = json.Marshal(reqParams)
			if err != nil {
				return err
			}
		}
		req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
		if err != nil {
			return err
		}

		req.Header.Add("Authorization", "Bearer "+config.Notion.NotionApiKey)
		req.Header.Add("Notion-Version", config.Notion.NotionVersion)
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Println("Error making request:", err)
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error reading response:", err)
			return err
		}

		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("got error:%s", string(respBody))
			return err
		}

		return json.Unmarshal(respBody, respStruct)
	},
		retry.Attempts(5),
		retry.Delay(2*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("Retry #%d to %s due to error: %s\n", n, url, err)
		}),
	)
}
