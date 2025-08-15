package victoria

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var client = &http.Client{}

func Report[T any](address string, entries []T) error {
	r, w := io.Pipe()
	go func() {
		enc := json.NewEncoder(w)
		for _, item := range entries {
			_ = enc.Encode(item)
		}
		w.Close()
	}()
	resp, err := client.Post(address+"/insert/jsonline?_stream_fields=gateway,test", "application/stream+json", r)
	if err != nil {
		return fmt.Errorf("error posting victoria logs: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error posting victoria logs: code %v", resp.StatusCode)
	}
	return nil
}
