package worker

import (
	"context"
	"testing"
	"time"

	"github.com/fermitools/managed-tokens/internal/service"
	"github.com/stretchr/testify/assert"
)

// TODO: See if we can test other cases
// TODO: See if we can test tokenGetterConfig.GetToken more directly

func TestGetTokenWorker(t *testing.T) {
	ctx := context.Background()

	// bad case
	t.Run("get token worker fails", func(t *testing.T) {
		t.Parallel()
		chans := NewChannelsForWorkers(1)
		t.Cleanup(func() {
			select {
			case _, ok := <-chans.GetSuccessChan():
				if ok {
					chans.closeWorkerSendChans()
				}
			default:
			}
		})
		s := service.NewService("testbad_service")
		sc, _ := NewConfig(s)
		chans.GetServiceConfigChan() <- sc
		close(chans.GetServiceConfigChan())
		go getTokenWorker(ctx, chans)
		select {
		case n := <-chans.GetNotificationsChan():
			assert.NotNil(t, n, "Expected notification on NotificationsChan, got nil")
			assert.Contains(t, n.GetMessage(), "could not store and get vault tokens")
			assert.Equal(t, n.GetService(), "testbad_service", "Expected service name to be 'testbad_service'")
		case <-time.After(10 * time.Second):
			t.Error("Expected notification on NotificationsChan, got none after 10 second timeout")
		}

		select {
		case s := <-chans.GetSuccessChan():
			assert.Equal(t, s.GetService().Name(), "testbad_service", "Expected service name to be 'testbad_service'")
			assert.False(t, s.GetSuccess(), "Expected success=false on getTokenSuccess, got success=true")
		case <-time.After(10 * time.Second):
			t.Error("Expected getTokenSuccess on SuccessChan, got none after 10 second timeout")
		}
	})

	// good case
	t.Run("get token worker successful", func(t *testing.T) {
		t.Parallel()
		s2 := service.NewService("testgood_service")
		chans2 := NewChannelsForWorkers(1)
		t.Cleanup(func() {
			select {
			case _, ok := <-chans2.GetSuccessChan():
				if ok {
					chans2.closeWorkerSendChans()
				}
			default:
			}
		})
		f := &fakeTokenGetter{}
		sc2, _ := NewConfig(s2, SetAlternateTokenGetterOption(GetToken, f))
		chans2.GetServiceConfigChan() <- sc2
		close(chans2.GetServiceConfigChan())
		go getTokenWorker(ctx, chans2)
		select {
		case n, ok := <-chans2.GetNotificationsChan():
			if ok || n != nil {
				t.Error("Channel should have been closed with no values received")
			}
		case <-time.After(10 * time.Second):
			t.Error("Expected closed NotificationsChan, got none after 10 second timeout")
		}

		select {
		case s := <-chans2.GetSuccessChan():
			assert.Equal(t, s.GetService().Name(), "testgood_service", "Expected service name to be 'testgood_service'")
			assert.True(t, s.GetSuccess(), "Expected success=true on getTokenSuccess, got success=false")
		case <-time.After(10 * time.Second):
			t.Error("Expected getTokenSuccess on SuccessChan, got none after 10 second timeout")

		}
	})
}

type fakeTokenGetter struct{}

func (f *fakeTokenGetter) GetToken(ctx context.Context) error {
	return nil
}
