import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func handler(req map[string]interface{}) map[string]interface{} {
	todoId := req["query"].(map[string]interface{})["id"]
	if todoId == nil {
		return map[string]interface{}{
			"error": "Missing 'id' query parameter",
		}
	}
	url := "https://jsonplaceholder.typicode.com/todos/" + fmt.Sprintf("%v", todoId)
	resp, err := http.Get(url)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Failed to fetch data from %s: %v", url, err),
		}
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Failed to read response body: %v", err),
		}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Failed to parse JSON response: %v", err),
		}
	}
	return map[string]interface{}{
		"message":   "Hello from Cubis Workers (Go)!",
		"method":    req["method"],
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"runtime":   "go",
		"query":     req["query"],
		"data":      data,
	}
}