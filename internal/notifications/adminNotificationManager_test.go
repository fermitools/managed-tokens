// COPYRIGHT 2024 FERMI NATIONAL ACCELERATOR LABORATORY
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
//
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package notifications

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/fermitools/managed-tokens/internal/db"
	"github.com/stretchr/testify/assert"
)

func TestNewAdminNotificationManagerDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })
	a := NewAdminNotificationManager(ctx)
	newAdminNotificationManagerTests(t, a, func(t *testing.T, anm *AdminNotificationManager) {
		assert.Equal(t, 0, a.NotificationMinimum)
	})
}

func TestNewAdminNotificationManagerFuncOpt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })
	funcOpt := AdminNotificationManagerOption(func(anm *AdminNotificationManager) error {
		anm.NotificationMinimum = 42
		return nil
	})
	a := NewAdminNotificationManager(ctx, funcOpt)
	newAdminNotificationManagerTests(t, a, func(t *testing.T, anm *AdminNotificationManager) {
		assert.Equal(t, 42, a.NotificationMinimum)
	})
}

func TestNewAdminNotificationManagerFuncOptWithError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })
	funcOpt := AdminNotificationManagerOption(func(anm *AdminNotificationManager) error {
		anm.NotificationMinimum = 42
		return errors.New("This is an error")
	})
	a := NewAdminNotificationManager(ctx, funcOpt)
	newAdminNotificationManagerTests(t, a,
		func(t *testing.T, anm *AdminNotificationManager) {
			t.Run(
				"Check that the value we tried changing, but got an error, was reset to the old value",
				func(t *testing.T) { assert.Equal(t, 0, a.NotificationMinimum) },
			)
		},
		func(t *testing.T, anm *AdminNotificationManager) {
			t.Run("check that we got a valid new AdminNotificationManager", func(t *testing.T) {
				newAdminNotificationManagerTests(t, a)
			})
		},
	)
}
func newAdminNotificationManagerTests(t *testing.T, a *AdminNotificationManager, extraTests ...func(*testing.T, *AdminNotificationManager)) {
	assert.Nil(t, a.Database)
	assert.True(t, a.DatabaseReadOnly)
	assert.False(t, a.TrackErrorCounts)
	assert.Equal(t, map[string]*serviceErrorCounts{}, a.allServiceCounts)
	assert.NotNil(t, a.receiveChan)
	assert.NotNil(t, a.adminErrorChan)

	for _, test := range extraTests {
		test(t, a)
	}
}

func TestBackupAdminNotificationManager(t *testing.T) {
	a1 := new(AdminNotificationManager)
	testBackupAdminNotificationManager(t, a1)
}

// copy a1 and make sure that we copy by value or initialize new fields appropriately
func testBackupAdminNotificationManager(t *testing.T, a1 *AdminNotificationManager) {
	a2 := backupAdminNotificationManager(a1)

	assert.Equal(t, a1.Database, a2.Database)
	assert.Equal(t, a1.NotificationMinimum, a2.NotificationMinimum)
	assert.Equal(t, a1.TrackErrorCounts, a2.TrackErrorCounts)
	assert.Equal(t, a1.DatabaseReadOnly, a2.DatabaseReadOnly)
	assert.Equal(t, a1.allServiceCounts, a2.allServiceCounts)

	assert.NotNil(t, a2.receiveChan)
	assert.NotNil(t, a2.adminErrorChan)

	// This is just a check to make sure that both of these fields are initialized and work properly
	assert.Eventually(t, func() bool {
		a2.closeReceiveChanOnce.Do(
			func() {
				a2.notificationSourceWg.Wait()
			},
		)
		return true
	}, time.Second, 10*time.Millisecond)
}

func TestDetermineIfShouldTrackErrorCounts(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	type testCase struct {
		description             string
		priorTrackErrorCounts   bool
		databaseSetupFunc       func() *db.ManagedTokensDatabase
		expectedShouldTrack     bool
		expectedServicesToTrack []string
	}

	testCases := []testCase{
		{
			"AdminNotificationManager field set to false",
			false,
			func() *db.ManagedTokensDatabase { return nil },
			false,
			nil,
		},
		{
			"nil database",
			true,
			func() *db.ManagedTokensDatabase { return nil },
			false,
			nil,
		},
		{
			"Couldn't open DB",
			true,
			func() *db.ManagedTokensDatabase {
				m, _ := db.OpenOrCreateDatabase(os.DevNull)
				return m
			},
			false,
			nil,
		},
		{
			"No services loaded",
			true,
			func() *db.ManagedTokensDatabase {
				m, _ := db.OpenOrCreateDatabase(path.Join(tmp, fmt.Sprintf("managed-tokens-test-%d.db", rand.Intn(10000))))
				return m
			},
			false,
			nil,
		},
		{
			"Good case - services loaded",
			true,
			func() *db.ManagedTokensDatabase {
				m, _ := db.OpenOrCreateDatabase(path.Join(tmp, fmt.Sprintf("managed-tokens-test-%d.db", rand.Intn(10000))))
				m.UpdateServices(ctx, []string{"service1", "service2", "service3"})
				return m
			},
			true,
			[]string{"service1", "service2", "service3"},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			a := new(AdminNotificationManager)
			a.TrackErrorCounts = test.priorTrackErrorCounts
			a.Database = test.databaseSetupFunc()
			if a.Database != nil {
				defer a.Database.Close()
			}
			shouldTrackErrors, servicesToTrack := determineIfShouldTrackErrorCounts(ctx, a)
			assert.Equal(t, test.expectedShouldTrack, shouldTrackErrors)
			assert.Equal(t, test.expectedServicesToTrack, servicesToTrack)

		})
	}
}

func TestGetAllErrorCountsFromDatabase(t *testing.T) {
	priorSetupErrors := []db.SetupErrorCount{
		&setupErrorCount{
			"service1",
			2,
		},
		&setupErrorCount{
			"service2",
			3,
		},
	}
	priorPushErrors := []db.PushErrorCount{
		&pushErrorCount{
			"service1",
			"node1",
			2,
		},
		&pushErrorCount{
			"service1",
			"node2",
			0,
		},
		&pushErrorCount{
			"service1",
			"node3",
			2,
		},
		&pushErrorCount{
			"service2",
			"node1",
			1,
		},
		&pushErrorCount{
			"service2",
			"node2",
			0,
		},
		&pushErrorCount{
			"service2",
			"node3",
			1,
		},
	}

	expectedServiceCounts := map[string]*serviceErrorCounts{

		"service1": {
			setupErrors: errorCount{2, false},
			pushErrors: map[string]errorCount{
				"node1": {2, false},
				"node2": {0, false},
				"node3": {2, false},
			},
		},
		"service2": {
			setupErrors: errorCount{3, false},
			pushErrors: map[string]errorCount{
				"node1": {1, false},
				"node2": {0, false},
				"node3": {1, false},
			},
		},
	}

	var err error
	a := new(AdminNotificationManager)
	tempDir := t.TempDir()
	services := []string{"service1", "service2"}
	nodes := []string{"node1", "node2", "node3"}
	a.Database, err = createAndPrepareDatabaseForTesting(tempDir, services, nodes, priorSetupErrors, priorPushErrors)
	if err != nil {
		t.Errorf("Error creating and preparing the database: %s", err)
	}
	defer a.Database.Close()

	errorCountsFromDb, ok := getAllErrorCountsFromDatabase(context.Background(), services, a.Database)
	assert.True(t, ok)
	assert.Equal(t, expectedServiceCounts, errorCountsFromDb)

}

func TestGetAllErrorCountsFromDatabaseFail(t *testing.T) {
	a := new(AdminNotificationManager)
	a.Database, _ = db.OpenOrCreateDatabase(os.DevNull)
	servicesToQuery := []string{"service1", "service2"}
	errorCountsFromDb, ok := getAllErrorCountsFromDatabase(context.Background(), servicesToQuery, a.Database)
	assert.False(t, ok)
	assert.Nil(t, errorCountsFromDb)
}

func TestRunAdminNotificationHandlerContextExpired(t *testing.T) {
	a := setupAdminNotificationManagerForHandlerTest()
	t.Cleanup(func() { close(a.receiveChan) })

	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})

	a.runAdminNotificationHandler(ctx)

	// Cancel our context, and indicate when runAdminNotificationHandler has returned
	go func() {
		cancel()
		<-a.adminErrorChan
		close(returned)
	}()

	// receiveChan should be open, and return should be closed
	assert.Eventually(t, func() bool {
		select {
		case <-returned:
			return true
		case <-a.receiveChan:
			t.Fatal("Context was canceled - a.receiveChan should be open and no values sent on this channel")
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)

}

func TestRunAdminNotificationHandler(t *testing.T) {
	a := setupAdminNotificationManagerForHandlerTest()
	a.TrackErrorCounts = true
	a.allServiceCounts = map[string]*serviceErrorCounts{
		"service1": {
			setupErrors: errorCount{
				0,
				false,
			},
		},
	}

	// Give ourselves 10 seconds for this to work
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(func() { cancel() })
	returned := make(chan struct{})

	a.runAdminNotificationHandler(ctx)

	t.Run(
		"If we send SourceNotification to AdminNotificationHandler, it should get forwarded on adminErrorChan",
		func(t *testing.T) {
			sn := SourceNotification{&setupError{"message1", "service1"}}

			// Sender
			go func() {
				a.receiveChan <- sn
			}()

			// Listener
			n := <-a.adminErrorChan
			assert.Equal(t, n, sn.Notification)
		},
	)

	t.Run(
		"If we send Notification, we get shouldSend = false and we don't send",
		func(t *testing.T) {
			n := &setupError{"message1", "service1"}

			senderDone := make(chan struct{})
			// Sender
			go func() {
				a.receiveChan <- n
				close(senderDone)
			}()

			// Listener
			select {
			case <-a.adminErrorChan:
				t.Fatal("We should not have sent this Notification on a.adminErrorChan")
			case <-senderDone:
				return
			}
		},
	)

	t.Run(
		"If we send another Notification, we get shouldSend = true and we forward the Notification on adminErrorChan",
		func(t *testing.T) {
			n := &setupError{"message2", "service1"}

			// Sender
			go func() {
				a.receiveChan <- n
			}()

			// Listener
			nReceived := <-a.adminErrorChan
			assert.Equal(t, n, nReceived)
		},
	)

	t.Run(
		"Now if we close receiveChan, we should return before the context expires",
		func(t *testing.T) {
			// close receiveChan, and indicate when runAdminNotificationHandler has returned
			go func() {
				close(a.receiveChan)
				<-a.adminErrorChan
				close(returned)
			}()

			select {
			case <-returned:
				return
			case <-ctx.Done():
				t.Fatal("Timeout for test.  We should have returned before the 10s timeout")
			}
		},
	)
}

func setupAdminNotificationManagerForHandlerTest() *AdminNotificationManager {
	a := &AdminNotificationManager{
		NotificationMinimum: 2,
		DatabaseReadOnly:    true,
	}
	a.receiveChan = make(chan Notification)
	a.adminErrorChan = make(chan Notification)
	return a
}
func TestRegisterNotificationSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := new(AdminNotificationManager)
	a.receiveChan = make(chan Notification)

	// Listener
	receiveDone := make(chan struct{})
	go func() {
		<-a.receiveChan
		close(receiveDone)
	}()

	// Sender
	sendChan := a.RegisterNotificationSource(ctx)
	go func() {
		defer close(sendChan)
		sendChan <- SourceNotification{NewSetupError("message", "service")}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Registration failed.  Did not receive from a.receiveChan within timeout")
	case <-receiveDone:
		return
	}
}

func TestRequestToCloseReceiveChanContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := new(AdminNotificationManager)

	a.notificationSourceWg.Add(1)
	returned := make(chan struct{})
	go func() {
		a.RequestToCloseReceiveChan(ctx)
		close(returned)
	}()
	cancel()
	select {
	case <-returned:
		return
	case <-a.receiveChan:
		t.Fatal("context cancel should have caused RequestToCloseReceiveChan to return without closing a.ReceiveChan")
	}
}

func TestRequestToCloseReceiveChan(t *testing.T) {
	ctx := context.Background()
	a := new(AdminNotificationManager)
	a.receiveChan = make(chan Notification)

	a.notificationSourceWg.Add(1)
	go a.RequestToCloseReceiveChan(ctx)
	go a.notificationSourceWg.Done()
	select {
	case <-ctx.Done():
		t.Fatal("context should not be canceled and receiveChan should be closed")
	case <-a.receiveChan:
		return
	}
}

func TestRequestToCloseReceiveChanMultiple(t *testing.T) {
	ctx := context.Background()
	a := new(AdminNotificationManager)
	a.receiveChan = make(chan Notification)

	var wg sync.WaitGroup
	defer wg.Wait()
	a.notificationSourceWg.Add(1)
	// We can request to close the channel 10 times, but we should only do it once.  We should not get any panics
	defer func() {
		v := recover()
		if v != nil {
			t.Fatalf("Recovered: %v.  FAIL:  We should not have tried to close the already-closed receiveChan", v)
		}
	}()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.RequestToCloseReceiveChan(ctx)
		}()
	}
	go a.notificationSourceWg.Done()
	select {
	case <-ctx.Done():
		t.Fatal("context should not be canceled and receiveChan should be closed")
	case <-a.receiveChan:
		return
	}
}

// Check that if notificationSourceWg never gets to 0, we don't close receiveChan
func TestRequestToCloseReceiveChanDeadlock(t *testing.T) {
	ctx := context.Background()
	a := new(AdminNotificationManager)
	a.receiveChan = make(chan Notification)

	a.notificationSourceWg.Add(1)
	go a.RequestToCloseReceiveChan(ctx)

	assert.Never(
		t,
		func() bool {
			// This function should never return
			select {
			case <-ctx.Done():
				t.Fatal("context should not be canceled and receiveChan should be closed")
			case <-a.receiveChan:
				return true // This is also a fatal condition, but we let the assert.Never call handle this case
			}
			return true
		}, 2*time.Second, 10*time.Millisecond)
}

func TestStartAdminErrorAdder(t *testing.T) {
	a := new(AdminNotificationManager)
	a.adminErrorChan = make(chan Notification)
	a.startAdminErrorAdder()

	a.adminErrorChan <- &setupError{"message", "service1"}
	a.adminErrorChan <- &pushError{"message", "service1", "node1"}
	close(a.adminErrorChan)
	adminErrors.writerCount.Wait()

	unSyncedAdminData := adminErrorsToAdminDataUnsync()
	service1Data, ok := unSyncedAdminData["service1"]
	if !ok {
		t.Fatal("service1 data not stored")
	}

	expectedSetupErrors := []string{"message"}
	expectedPushErrors := map[string]string{"node1": "message"}

	assert.Equal(t, expectedSetupErrors, service1Data.SetupErrors)
	assert.Equal(t, expectedPushErrors, service1Data.PushErrors)

}

func TestVerifyServiceErrorCounts(t *testing.T) {
	type testCase struct {
		description string
		*AdminNotificationManager
		expectedServiceErrorCounts *serviceErrorCounts
		expectedReturnVal          bool
	}

	testCases := []testCase{
		{
			"pre-populated struct",
			&AdminNotificationManager{
				allServiceCounts: map[string]*serviceErrorCounts{
					"service1": {
						setupErrors: errorCount{
							4,
							false,
						},
					},
				},
			},
			&serviceErrorCounts{
				setupErrors: errorCount{
					4,
					false,
				},
			},
			true,
		},
		{
			"service missing for struct",
			&AdminNotificationManager{
				allServiceCounts: map[string]*serviceErrorCounts{
					"service2": {
						setupErrors: errorCount{
							0,
							false,
						},
					},
				},
			},
			&serviceErrorCounts{
				pushErrors: make(map[string]errorCount),
			},
			false,
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				retVal := test.AdminNotificationManager.verifyServiceErrorCounts("service1")
				assert.Equal(t, test.expectedReturnVal, retVal)
				assert.Equal(t, test.expectedServiceErrorCounts, test.AdminNotificationManager.allServiceCounts["service1"])
			},
		)
	}
}
