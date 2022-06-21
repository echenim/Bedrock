package models

import (
	"net"
	"sync/atomic"
)

var (
	ReadThroughput  int64
	WriteThroughput int64
)

type Conn struct {
	net.Conn
}

func (c *Conn) Read(b []byte) (n int, err error) {
	len, err := c.Conn.Read(b)

	if err == nil {
		atomic.AddInt64(&ReadThroughput, int64(len))
	}

	return len, err
}

func (c *Conn) Write(b []byte) (n int, err error) {
	len, err := c.Conn.Write(b)

	if err == nil {
		atomic.AddInt64(&WriteThroughput, int64(len))
	}

	return len, err
}
