package models

type Result struct {
	requests      int64
	success       int64
	networkFailed int64
	badFailed     int64
}
