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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ServiceEmailManager tests
func TestNewServiceEmailManagerDefault(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	service := "my_service"
	e := NewEmail("from_address", []string{"to_address"}, "test_subject", "smtp.host", 12345)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	s := NewServiceEmailManager(ctx, &wg, service, e)
	newServiceEmailManagerTests(t, s)

}

func TestNewServiceEmailManagerFuncOpt(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	service := "my_service"
	e := NewEmail("from_address", []string{"to_address"}, "test_subject", "smtp.host", 12345)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	s := NewServiceEmailManager(ctx, &wg, service, e,
		ServiceEmailManagerOption(func(sem *ServiceEmailManager) error {
			sem.NotificationMinimum = 42
			return nil
		},
		))
	newServiceEmailManagerTests(t, s, func(t *testing.T, sem *ServiceEmailManager) { assert.Equal(t, 42, sem.NotificationMinimum) })
}

func TestNewServiceEmailManagerFuncOptError(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	service := "my_service"
	e := NewEmail("from_address", []string{"to_address"}, "test_subject", "smtp.host", 12345)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	s := NewServiceEmailManager(ctx, &wg, service, e,
		ServiceEmailManagerOption(func(sem *ServiceEmailManager) error {
			sem.NotificationMinimum = 42
			return errors.New("This is an error")
		},
		))
	newServiceEmailManagerTests(t, s,
		func(t *testing.T, sem *ServiceEmailManager) {
			t.Run("Test that the effect of our funcOpt got rolled back", func(t *testing.T) {
				assert.Equal(t, 0, sem.NotificationMinimum)
			})
		},
		func(t *testing.T, sem *ServiceEmailManager) {
			t.Run("Test that our backed up ServiceEmailManager is valid", func(t *testing.T) {
				newServiceEmailManagerTests(t, sem)
			})
		},
	)
}

func newServiceEmailManagerTests(t *testing.T, s *ServiceEmailManager, extraTests ...func(*testing.T, *ServiceEmailManager)) {
	assert.Equal(t, "my_service", s.Service)
	assert.NotNil(t, s.ReceiveChan)
	assert.NotNil(t, s.Email)
	assert.NotNil(t, s.AdminNotificationManager)
	assert.NotNil(t, s.adminNotificationChan)
	assert.NotNil(t, s.wg)
	assert.False(t, s.trackErrorCounts)
	assert.Nil(t, s.errorCounts)

	for _, extraTest := range extraTests {
		extraTest(t, s)
	}
}
func TestBackupServiceEmailManager(t *testing.T) {
	s := new(ServiceEmailManager)
	s.Service = "test_service"
	s.Email = NewEmail("from_test", []string{"to1", "to2"}, "test_subject", "smtpHost", 12345)
	s.AdminNotificationManager = new(AdminNotificationManager)
	s.AdminNotificationManager.TrackErrorCounts = true // Note that this is a misconfiguration, but it's just there to make sure we carry the value in the backup copy
	s.NotificationMinimum = 42
	s.wg = &sync.WaitGroup{}
	s.trackErrorCounts = true
	s.errorCounts = &serviceErrorCounts{setupErrors: errorCount{4, true}}
	testBackupServiceEmailManager(t, s)
}

func TestBackupServiceEmailManagerNilPointers(t *testing.T) {
	s := new(ServiceEmailManager)
	testBackupServiceEmailManager(t, s)
}

func testBackupServiceEmailManager(t *testing.T, s1 *ServiceEmailManager) {
	s2 := backupServiceEmailManager(s1)

	assert.Equal(t, s1.Service, s2.Service)
	assert.Equal(t, s1.Email, s2.Email)
	assert.Equal(t, s1.AdminNotificationManager, s2.AdminNotificationManager)
	assert.Equal(t, s1.NotificationMinimum, s2.NotificationMinimum)
	assert.Equal(t, s1.wg, s2.wg)
	assert.Equal(t, s1.trackErrorCounts, s2.trackErrorCounts)
	assert.Equal(t, s1.errorCounts, s2.errorCounts)

	assert.NotNil(t, s2.adminNotificationChan)

	// Check that we get a valid new ReceiveChan that can actually receive
	go func() {
		s2.ReceiveChan <- &setupError{"this is a test message", "test_service"}
		close(s2.ReceiveChan)
	}()
	assert.Eventually(t, func() bool {
		chanVal := <-s2.ReceiveChan
		return assert.Equal(t, "this is a test message", chanVal.GetMessage())
	}, 10*time.Second, 10*time.Millisecond)
}

// runServiceNotificationHandler tests
func TestRunServiceNotificationHandlerContextExpired(t *testing.T) {
	s, _ := setupServiceEmailManagerForHandlerTest()
	t.Cleanup(func() { close(s.ReceiveChan) })

	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})

	s.wg.Add(1)
	s.runServiceNotificationHandler(ctx)

	// Cancel our context, and indicate when runAdminNotificationHandler has returned
	go func() {
		cancel()
		s.wg.Wait()
		close(returned)
	}()

	// receiveChan should be open, and return should be closed
	assert.Eventually(t, func() bool {
		select {
		case <-returned:
			return true
		case <-s.ReceiveChan:
			t.Fatal("Context was canceled - s.ReceiveChan should be open and no values sent on this channel")
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)

}
func TestRunServiceNotificationHandlerMessagesSent(t *testing.T) {
	// Note:  right now there are no cases for which doIfReturned actually does anything.  But I've added it here in case we need to populate
	// that in the future
	type testCase struct {
		description        string
		notificationSender func() chan Notification
		doIfReturned       func(*testing.T, *ServiceEmailManager)
		doIfContextDone    func(*testing.T)
		doIfSentMessage    func(*testing.T, *ServiceEmailManager)
	}

	// A quick wrapper that returns a func that makes a channel, does f to it concurrently, and returns a func that returns the channel
	senderWrapper := func(f func(chan Notification)) func() chan Notification {
		return func() chan Notification {
			c := make(chan Notification)
			go f(c)
			return c
		}
	}

	testCases := []testCase{
		{
			"No errors",
			senderWrapper(func(c chan Notification) {
				close(c)
			}),
			func(*testing.T, *ServiceEmailManager) {},
			func(t *testing.T) { t.Fatal("Context timed out.  The function should have returned normally") },
			func(t *testing.T, s *ServiceEmailManager) {
				t.Fatal("Should not have sent any email here - there were no errors")
			},
		},
		{
			"Only a setup error",
			senderWrapper(func(c chan Notification) {
				c <- NewSetupError("This is a setup error", "myservice")
				close(c)
			}),
			func(*testing.T, *ServiceEmailManager) {},
			func(t *testing.T) { t.Fatal("Context timed out.  The function should have returned normally") },
			func(t *testing.T, s *ServiceEmailManager) {
				t.Fatal("Should not have sent any email here - we only sent a SetupError")
			},
		},
		{
			"Only a push error",
			senderWrapper(func(c chan Notification) {
				c <- NewPushError("This is a push error", "myservice", "mynode")
				close(c)
			}),
			func(*testing.T, *ServiceEmailManager) {},
			func(t *testing.T) { t.Fatal("Context timed out.  The function should have returned normally") },
			func(t *testing.T, s *ServiceEmailManager) {
				assert.Eventually(t, func() bool {
					s.wg.Wait()
					return true
				}, 10*time.Second, 10*time.Millisecond)
			},
		},
		{
			"Both Setup and Push Errors",
			senderWrapper(func(c chan Notification) {
				c <- NewSetupError("This is a setup error", "myservice")
				c <- NewPushError("This is a setup error", "myservice", "mynode")
				close(c)
			}),
			func(*testing.T, *ServiceEmailManager) {},
			func(t *testing.T) { t.Fatal("Context timed out.  The function should have returned normally") },
			func(t *testing.T, s *ServiceEmailManager) {
				assert.Eventually(t, func() bool {
					s.wg.Wait()
					return true
				}, 10*time.Second, 10*time.Millisecond)
			},
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				s, c := setupServiceEmailManagerForHandlerTest()

				// We need to type check here so we have access to the sentEmail channel of s.Email
				f, ok := s.Email.(*goodEmail)
				if !ok {
					t.Fatal("Used wrong type for fake SendMessager object")
				}

				// Drain the fake adminNotificationChan c
				go func() {
					for range c {
					}
				}()

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.Cleanup(func() { cancel() })

				// This chan gets closed when runServiceNotificationHandler returns
				returned := make(chan struct{})
				go func() {
					s.wg.Wait()
					close(returned)
				}()

				// Run our handler
				s.wg.Add(1)
				s.runServiceNotificationHandler(ctx)

				// Send whatever the notificationSender func specifies
				go func() {
					c := test.notificationSender()
					for n := range c {
						s.ReceiveChan <- n
					}
					close(s.ReceiveChan)
				}()

				select {
				case <-returned:
					// The reason why we check test.doIfSentMessage is because
					// 1.  If the message should have sent, this check will ensure that if this branch is selected over the <-f.sentMessage branch,
					//     the message was actually sent
					// 2.  If the message should NOT have sent, the test.doIfSentMessage check should be calling a t.Fatal(), failing the test
					select {
					case <-f.sentMessage:
						test.doIfSentMessage(t, s)
					default:
						test.doIfReturned(t, s)
					}
				case <-ctx.Done():
					test.doIfContextDone(t)
				case <-f.sentMessage:
					test.doIfSentMessage(t, s)
				}
			},
		)
	}
}

// This function returns a *ServiceEmailManager that is ready for testing (s), along with a chan SourceNotification (fakeAdminNotificationChan)
// In most cases during testing, the caller will need to drain or otherwise start up a goroutine that listens on fakeAdminNotificationChan,
// otherwise the serviceNotificationHandler will be blocked on trying to send on fakeAdminNotificationHandler.  This can be accomplished
// by doing the following before the handler is started:
//
//	s, c := setupServiceEmailManagerForHandlerTest()
//	// Drain the fake adminNotificationChan c
//	go func() {
//		for range c {
//		}
//	}()
//	go sendValuesOnSReceiveChan()
//	s.runServiceNotificationHandler(ctx)
func setupServiceEmailManagerForHandlerTest() (s *ServiceEmailManager, fakeAdminNotificationChan chan SourceNotification) {
	s = &ServiceEmailManager{
		NotificationMinimum: 2,
		Service:             "myservice",
		Email:               &goodEmail{make(chan struct{})},
	}
	s.ReceiveChan = make(chan Notification)
	s.wg = new(sync.WaitGroup)
	s.errorCounts = &serviceErrorCounts{pushErrors: make(map[string]errorCount)}

	c := make(chan SourceNotification)
	s.adminNotificationChan = c
	return s, c
}

// fakeNotification is a Notification that we use to make sure that type checking code works properly
type fakeNotification struct{}

func (f *fakeNotification) GetMessage() string { return "" }
func (f *fakeNotification) GetService() string { return "" }

func TestAddPushErrorNotificationToServiceErrorsTable(t *testing.T) {
	type testCase struct {
		description   string
		previousTable map[string]string
		nToAdd        Notification
		expectedTable map[string]string
	}

	testCases := []testCase{
		{
			"No previous errors, add setup error",
			make(map[string]string),
			NewSetupError("This is a setup error", "myservice"),
			map[string]string{},
		},
		{
			"No previous errors, add push error",
			make(map[string]string),
			NewPushError("This is a push error", "myservice", "mynode"),
			map[string]string{"mynode": "This is a push error"},
		},
		{
			"Previous errors, add setup error",
			map[string]string{"mynode1": "This is a push error"},
			NewSetupError("This is a setup error", "myservice"),
			map[string]string{"mynode1": "This is a push error"},
		},
		{
			"Previous errors, add push error",
			map[string]string{"mynode1": "This is a push error"},
			NewPushError("This is a push error as well", "myservice", "mynode2"),
			map[string]string{
				"mynode1": "This is a push error",
				"mynode2": "This is a push error as well",
			},
		},
		{
			"Previous errors, add fake notification",
			map[string]string{"mynode1": "This is a push error"},
			&fakeNotification{},
			map[string]string{
				"mynode1": "This is a push error",
			},
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				addPushErrorNotificationToServiceErrorsTable(test.nToAdd, test.previousTable)
				assert.Equal(t, test.expectedTable, test.previousTable)
			},
		)
	}
}

func TestSendServiceEmailIfErrors(t *testing.T) {
	type testCase struct {
		description string
		tableToSend map[string]string
		em          *ServiceEmailManager
		doIfSent    func(*testing.T)
		doIfNotSent func(*testing.T)
	}

	testCases := []testCase{
		{
			"Empty serviceErrorsTable",
			make(map[string]string),
			&ServiceEmailManager{Email: &goodEmail{make(chan struct{})}},
			func(t *testing.T) { t.Fatal("No message should have been sent") },
			func(*testing.T) {},
		},
		{
			"Email gets sent successfully",
			map[string]string{"node1": "error1"},
			&ServiceEmailManager{Email: &goodEmail{make(chan struct{})}},
			func(*testing.T) {},
			func(t *testing.T) { t.Fatal("Message should have been sent") },
		},
		{
			"Problem sending email",
			map[string]string{"node1": "error1"},
			&ServiceEmailManager{Email: &badEmail{make(chan struct{})}},
			func(t *testing.T) { t.Fatal("No message should have been sent") },
			func(*testing.T) {},
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.Cleanup(func() { cancel() })

				sendServiceEmailIfErrors(ctx, test.tableToSend, test.em)

				// Need to do this to gain access to sentEmail chan of underlying type of SendMessager
				var emailSentChan chan struct{}
				switch v := test.em.Email.(type) {
				case *goodEmail:
					emailSentChan = v.sentMessage
				case *badEmail:
					emailSentChan = v.sentMessage
				default:
					t.Fatal("Unsupported underlying SendMessager type for this set of tests.  Please use *goodEmail or *badEmail")
				}

				t.Cleanup(func() {
					select {
					case <-emailSentChan:
					default:
						close(emailSentChan)
					}
				})

				select {
				case <-ctx.Done():
					t.Fatal("Timeout")
				case <-emailSentChan:
					test.doIfSent(t)
				default:
					test.doIfNotSent(t)
				}
			},
		)
	}
}

// Note: No test exists for prepareServiceEmail, since that just sets a couple of values and then calls prepareMessageFromTemplate, a tested function,
// to handle any further logic

type goodEmail struct {
	sentMessage chan struct{}
}

func (g *goodEmail) sendMessage(ctx context.Context, message string) error {
	close(g.sentMessage)
	return nil
}

type badEmail struct {
	sentMessage chan struct{}
}

func (b badEmail) sendMessage(ctx context.Context, message string) error {
	return errors.New("Failed to send message")
}
