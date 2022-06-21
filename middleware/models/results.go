package models

type Result struct {
	Requests      int64
	Success       int64
	NetworkFailed int64
	BadFailed     int64
}
