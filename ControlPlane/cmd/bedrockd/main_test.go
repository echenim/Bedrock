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
	cmd := startCmd()
	if cmd.Use != "start" {
		t.Errorf("expected Use='start', got '%s'", cmd.Use)
	}
}

func TestInitCmd(t *testing.T) {
	cmd := initCmd()
	if cmd.Use != "init [moniker]" {
		t.Errorf("expected Use='init [moniker]', got '%s'", cmd.Use)
	}
}
