package models

import (
	"github.com/valyala/fasthttp"
)

type Configuration struct {
	Urls       []string
	Method     string
	PostData   []byte
	Requests   int64
	Period     int64
	KeepAlive  bool
	AuthHeader string

	Client fasthttp.Client
}
