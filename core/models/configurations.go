package models

import (
	"github.com/valyala/fasthttp"
)

type Configuration struct {
	urls       []string
	method     string
	postData   []byte
	requests   int64
	period     int64
	keepAlive  bool
	authHeader string

	client fasthttp.Client
}
