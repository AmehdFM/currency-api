package models

type APIResponse struct {
	Success   bool               `json:"success"`
	Timestamp int64              `json:"timestamp"`
	Quotes    map[string]float64 `json:"quotes"`
}
