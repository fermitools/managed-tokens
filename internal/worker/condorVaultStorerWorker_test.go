package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fermitools/managed-tokens/internal/service"
)

func TestStoreAndGetTokenWorker(t *testing.T) {
	ctx := context.Background()
	schedd := "test_schedd"
	vaultServer := "test_vault_server"

	// bad case
	t.Run("store and get token worker fails", func(t *testing.T) {
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

		f := &fakeTokenStorerAndGetter{t: t, credd: schedd, vaultServer: vaultServer, shouldFail: true}

		s := service.NewService("testbad_service")
		sc, _ := NewConfig(s, SetSchedds([]string{"bad_schedd"}), SetAlternateTokenStorerAndGetterOption(StoreAndGetToken, f))
		chans.GetServiceConfigChan() <- sc
		close(chans.GetServiceConfigChan())
		go storeAndGetTokenWorker(ctx, chans)
		select {
		case n := <-chans.GetNotificationsChan():
			assert.NotNil(t, n, "Expected notification on NotificationsChan, got nil")
			assert.Contains(t, n.GetMessage(), "Could not store and get vault tokens")
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

	// Good case
	t.Run("store and get token worker succeeds", func(t *testing.T) {
		t.Parallel()

		f := &fakeTokenStorerAndGetter{t: t, credd: schedd, vaultServer: vaultServer, shouldFail: false}
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
		sc2, _ := NewConfig(s2, SetAlternateTokenStorerAndGetterOption(StoreAndGetToken, f))
		chans2.GetServiceConfigChan() <- sc2
		close(chans2.GetServiceConfigChan())
		go storeAndGetTokenWorker(ctx, chans2)
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
			assert.True(t, s.GetSuccess(), "Expected success=true on storeAndGetTokenSuccess, got success=false")
		case <-time.After(10 * time.Second):
			t.Error("Expected storeAndGetTokenSuccess on SuccessChan, got none after 10 second timeout")

		}
	})
}

type fakeTokenStorerAndGetter struct {
	t           *testing.T
	credd       string
	vaultServer string
	shouldFail  bool
}

func (f *fakeTokenStorerAndGetter) GetAndStoreToken(ctx context.Context, serviceName string, interactive bool) error {
	f.t.Log("Using fakeTokenStorerAndGetter")
	if f.shouldFail {
		return errors.New("simulated error")
	}
	return nil
}

func (f *fakeTokenStorerAndGetter) GetCredd() string       { return f.credd }
func (f *fakeTokenStorerAndGetter) GetVaultServer() string { return f.vaultServer }
