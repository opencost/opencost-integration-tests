package allocation

import (
    "encoding/json"
    "net/http"
    "testing"
)

func TestNegativeIdle(t *testing.T) {
    url := "https://demo.infra.opencost.io/model/allocation/compute?window=1d&aggregate=namespace&includeIdle=true&step=1d&accumulate=false"
    resp, err := http.Get(url)
    if err != nil {
        t.Fatal("Failed to query API: ", err.Error()) // Avoid %v
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatal("Unexpected status code: ", resp.StatusCode)
    }

    var response struct {
        Data []map[string]interface{} `json:"data"` // Fixed tag
    }
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        t.Fatal("Failed to parse JSON: ", err.Error()) // Avoid %v
    }

    for _, allocSet := range response.Data {
        if idle, ok := allocSet["__idle__"].(map[string]interface{}); ok {
            if cpuIdle, ok := idle["cpuCoreIdleHours"].(float64); ok && cpuIdle < 0 {
                t.Errorf("Negative cpuCoreIdleHours in __idle__: %f", cpuIdle)
            }
            if ramIdle, ok := idle["ramByteIdleHours"].(float64); ok && ramIdle < 0 {
                t.Errorf("Negative ramByteIdleHours in __idle__: %f", ramIdle)
            }
        }
    }
}