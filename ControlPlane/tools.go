//go:build tools

package controlplane

// This file ensures that module dependencies required by the project
// infrastructure (but not yet imported in application code) are tracked
// in go.mod. These will be imported directly as implementation proceeds.
//
// See: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/bytecodealliance/wasmtime-go/v29"
	_ "github.com/cockroachdb/pebble"
	_ "github.com/libp2p/go-libp2p"
	_ "github.com/pelletier/go-toml/v2"
	_ "github.com/prometheus/client_golang/prometheus"
	_ "go.uber.org/zap"
	_ "google.golang.org/grpc"
	_ "google.golang.org/protobuf/proto"
)
