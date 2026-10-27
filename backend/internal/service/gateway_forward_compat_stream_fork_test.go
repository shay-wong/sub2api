//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatDisconnectDrainDeadlineSurvivesHeartbeats(t *testing.T) {
	original := anthropicCompatDisconnectDrainInterval
	anthropicCompatDisconnectDrainInterval = 100 * time.Millisecond
	t.Cleanup(func() { anthropicCompatDisconnectDrainInterval = original })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	stream := newAnthropicCompatStream(nil, c, &http.Response{Body: reader}, "test", "request")
	defer stream.stop()
	stream.markClientDisconnected()
	written := make(chan struct{})
	go func() {
		defer close(written)
		for {
			if _, err := io.WriteString(writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n"); err != nil {
				return
			}
		}
	}()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			if _, _, ok := stream.nextEvent(); !ok {
				return
			}
		}
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("upstream heartbeats prolonged the disconnected stream past its total deadline")
	}
	<-written
	require.True(t, stream.clientDisconnected)
}
