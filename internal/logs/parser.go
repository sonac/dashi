package logs

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"time"

	"dashi/internal/models"
)

func ParseDockerStream(r io.Reader, serviceID, containerID string, out chan<- models.LogEntry) error {
	br := bufio.NewReader(r)
	for {
		header, err := br.Peek(8)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return parsePlainStream(br, serviceID, containerID, out)
			}
			return err
		}
		// Docker uses an 8-byte multiplex header when container TTY is disabled.
		if !isMultiplexHeader(header) {
			return parsePlainStream(br, serviceID, containerID, out)
		}
		_, _ = br.Discard(8)
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		}
		size := binary.BigEndian.Uint32(header[4:])
		if size == 0 {
			continue
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(br, payload); err != nil {
			return err
		}
		emitEntry(string(payload), stream, serviceID, containerID, out)
	}
}

func isMultiplexHeader(header []byte) bool {
	if len(header) < 8 {
		return false
	}
	if header[0] != 1 && header[0] != 2 {
		return false
	}
	return header[1] == 0 && header[2] == 0 && header[3] == 0
}

func parsePlainStream(br *bufio.Reader, serviceID, containerID string, out chan<- models.LogEntry) error {
	sc := bufio.NewScanner(br)
	for sc.Scan() {
		emitEntry(sc.Text(), "stdout", serviceID, containerID, out)
	}
	return sc.Err()
}

func emitEntry(raw, stream, serviceID, containerID string, out chan<- models.LogEntry) {
	msg := strings.TrimSpace(raw)
	ts := time.Now().UTC()
	if p := strings.SplitN(msg, " ", 2); len(p) == 2 {
		if t, err := time.Parse(time.RFC3339Nano, p[0]); err == nil {
			ts = t.UTC()
			msg = p[1]
		}
	}
	out <- models.LogEntry{
		TS:          ts,
		ServiceID:   serviceID,
		ContainerID: containerID,
		Level:       inferLevel(msg),
		Stream:      stream,
		Message:     sanitizeMessage(msg),
	}
}

func inferLevel(msg string) string {
	u := strings.ToUpper(msg)
	switch {
	case strings.Contains(u, "ERROR"), strings.Contains(u, "FATAL"), strings.Contains(u, "PANIC"):
		return "ERROR"
	case strings.Contains(u, "WARN"):
		return "WARN"
	case strings.Contains(u, "DEBUG"):
		return "DEBUG"
	default:
		return "INFO"
	}
}

func sanitizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.ReplaceAll(msg, "\x00", "")
	msg = strings.TrimSpace(string(bytes.ToValidUTF8([]byte(msg), []byte("?"))))
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	return msg
}
