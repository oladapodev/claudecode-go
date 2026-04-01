package shared

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type SSEEvent struct {
	Event string
	Data  []byte
}

func StreamSSE(ctx context.Context, client HTTPClient, req *http.Request, fn func(SSEEvent) error) error {
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		return fmt.Errorf("upstream error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventName string
	var data bytes.Buffer

	flush := func() error {
		if data.Len() == 0 {
			eventName = ""
			return nil
		}
		payload := bytes.TrimSuffix(data.Bytes(), []byte("\n"))
		err := fn(SSEEvent{Event: eventName, Data: payload})
		eventName = ""
		data.Reset()
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			data.WriteByte('\n')
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return flush()
}
