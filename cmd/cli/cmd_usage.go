package main

import (
	"encoding/json"
	"fmt"
)

func cmdUsage() {
	creds, err := loadCredentials()
	if err != nil {
		fatal(err.Error())
	}

	resp, err := apiRequest("GET", creds.APIURL+"/api/v1/account/usage", creds.Token, nil)
	if err != nil {
		fatal(err.Error())
	}
	if !resp.Success {
		fatal(resp.Error)
	}

	var data struct {
		Quota struct {
			Plan               string `json:"plan"`
			MaxRequestsPerDay  int64  `json:"max_requests_per_day"`
			MaxExecTimePerDay  int64  `json:"max_exec_time_per_day"`
			MaxBandwidthPerDay int64  `json:"max_bandwidth_per_day"`
			MaxWorkers         int    `json:"max_workers"`
		} `json:"quota"`
		Usage *struct {
			RequestCount   int64 `json:"request_count"`
			TotalExecTime  int64 `json:"total_exec_time"`
			TotalBandwidth int64 `json:"total_bandwidth"`
			ErrorCount     int64 `json:"error_count"`
		} `json:"usage"`
	}
	json.Unmarshal(resp.Data, &data)

	q := data.Quota
	fmt.Printf("Plan: %s\n\n", q.Plan)

	reqUsed := int64(0)
	execUsed := int64(0)
	bwUsed := int64(0)
	errCount := int64(0)
	if data.Usage != nil {
		reqUsed = data.Usage.RequestCount
		execUsed = data.Usage.TotalExecTime / 1000 // ms → s
		bwUsed = data.Usage.TotalBandwidth
		errCount = data.Usage.ErrorCount
	}

	fmt.Printf("%-22s %s\n", "Requests today:", fmtUsage(reqUsed, q.MaxRequestsPerDay, ""))
	fmt.Printf("%-22s %s\n", "Exec time today:", fmtUsage(execUsed, q.MaxExecTimePerDay, "s"))
	fmt.Printf("%-22s %s\n", "Bandwidth today:", fmtBytes(bwUsed)+" / "+fmtBytes(q.MaxBandwidthPerDay))
	fmt.Printf("%-22s %d\n", "Workers:", q.MaxWorkers)
	fmt.Printf("%-22s %d\n", "Errors today:", errCount)
}

func fmtUsage(used, max int64, unit string) string {
	if max <= 0 {
		return fmt.Sprintf("%d%s (unlimited)", used, unit)
	}
	pct := float64(used) / float64(max) * 100
	return fmt.Sprintf("%d%s / %d%s (%.0f%%)", used, unit, max, unit, pct)
}
