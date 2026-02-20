package main

import (
	"testing"
)

func TestVersionCmd(t *testing.T) {
	cmd := versionCmd()
	if cmd.Use != "version" {
		t.Errorf("expected Use='version', got '%s'", cmd.Use)
	}
}

func TestStartCmd(t *testing.T) {
	cmd := newStartCmd()
	if cmd.Use != "start" {
		t.Errorf("expected Use='start', got '%s'", cmd.Use)
	}
}

func TestInitCmd(t *testing.T) {
	cmd := newInitCmd()
	if cmd.Use != "init [moniker]" {
		t.Errorf("expected Use='init [moniker]', got '%s'", cmd.Use)
	}
}

func TestKeysCmd(t *testing.T) {
	cmd := newKeysCmd()
	if cmd.Use != "keys" {
		t.Errorf("expected Use='keys', got '%s'", cmd.Use)
	}
}

func TestDefaultHome(t *testing.T) {
	home := defaultHome()
	if home == "" {
		t.Error("expected non-empty default home")
	}
}
