// COPYRIGHT 2025 FERMI NATIONAL ACCELERATOR LABORATORY
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

package main

import (
	"bytes"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/fermitools/managed-tokens/internal/notifications"
	"github.com/fermitools/managed-tokens/internal/service"
)

// This test verifies that the proper routing is done.  We check for the error logged by the actual notifications.email.sendMessage() call, to make sure
// that a message was attempted to be sent. It's not as thorough as a test as I'd like at the moment, but it's better than nothing
func TestNotificationsManagerSendsMessage(t *testing.T) {
	// TODO:  We can test the case where we have no messages if we encapsulate the global routing chans into a type. Right now, the cleanup funcs at the
	// end close all the routing channels, so we can't reuse them

	// Set logger to write to our own bytes.Buffer
	var buf bytes.Buffer
	oldOut := log.StandardLogger().Out
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(oldOut)
	}()

	serviceName := "fakeexperiment_fakerole"
	// Configuration
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })
	viper.Set("email.from", "blah@example.com")
	viper.Set("experiments.fakeexperiment.emails", []string{"blah2@example.com"})
	viper.Set("email.smtphost", "smtp.example.com")
	viper.Set("email.smtpport", 42)

	// Needed components
	ctx := t.Context()
	s := service.NewService(serviceName)
	nChan := make(chan notifications.Notification) // We will send our notifications on this channel

	// Notifications setup
	// We're ignoring the notifications.AdminNotificationManager for this example by passing nil for the third argument
	registerServiceNotificationsChan(ctx, s, nil)
	startListenerOnWorkerNotificationChans(ctx, nChan)

	// Send a couple of notifications
	nChan <- notifications.NewSetupError("This is an error", serviceName)
	nChan <- notifications.NewPushError("This is a push error", serviceName, "fakenode")

	// Cleanup
	close(nChan)
	handleNotificationsFinalization()

	// Now check the buffer for our expected error string
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Error sending email")
}
