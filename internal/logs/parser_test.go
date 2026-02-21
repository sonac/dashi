package logs

import (
	"bytes"
	"encoding/binary"
	"testing"

	"dashi/internal/models"
)

func TestInferLevel(t *testing.T) {
	if got := inferLevel("fatal error happened"); got != "ERROR" {
		t.Fatalf("got %s", got)
	}
	if got := inferLevel("warn: bad"); got != "WARN" {
		t.Fatalf("got %s", got)
	}
	if got := inferLevel("debug details"); got != "DEBUG" {
		t.Fatalf("got %s", got)
	}
	if got := inferLevel("hello"); got != "INFO" {
		t.Fatalf("got %s", got)
	}
}

func TestParseDockerStream(t *testing.T) {
	payload := []byte("2026-01-01T00:00:00Z hello world\n")
	head := make([]byte, 8)
	head[0] = 1
	binary.BigEndian.PutUint32(head[4:], uint32(len(payload)))
	buf := append(head, payload...)

	out := make(chan models.LogEntry, 4)
	if err := ParseDockerStream(bytes.NewReader(buf), "svc", "cid", out); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	close(out)
	entry := <-out
	if entry.ServiceID != "svc" || entry.ContainerID != "cid" || entry.Level != "INFO" || entry.Message != "hello world" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}
