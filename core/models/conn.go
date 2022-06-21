package models

import (
	"net"
	"sync/atomic"
)

var (
	readThroughput  int64
	writeThroughput int64
)

type Conn struct {
	net.Conn
}

func (this *Conn) Read(b []byte) (n int, err error) {
	len, err := this.Conn.Read(b)

	if err == nil {
		atomic.AddInt64(&readThroughput, int64(len))
	}

	return len, err
}

func (this *Conn) Write(b []byte) (n int, err error) {
	len, err := this.Conn.Write(b)

	if err == nil {
		atomic.AddInt64(&writeThroughput, int64(len))
	}

	return len, err
}
