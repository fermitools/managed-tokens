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
	"fmt"
	"math/rand"
	"path"
	"testing"

	"github.com/fermitools/managed-tokens/internal/db"
	"github.com/stretchr/testify/assert"
)

func TestSetDatabaseOption(t *testing.T) {
	a := new(AdminNotificationManager)
	tempDir := t.TempDir()
	dbLocation := path.Join(tempDir, fmt.Sprintf("managed-tokens-test-%d.db", rand.Intn(10000)))
	database, err := db.OpenOrCreateDatabase(dbLocation)
	if err != nil {
		t.Fatal("Could not create database for testing")
	}
	funcOpt := SetAdminNotificationManagerDatabase(a, database)
	funcOpt(a)
	assert.Equal(t, database, a.Database)
}

func TestSetNotificationMinimum(t *testing.T) {
	a := new(AdminNotificationManager)
	notificationMinimum := 42
	funcOpt := SetNotificationMinimum(a, notificationMinimum)
	funcOpt(a)
	assert.Equal(t, notificationMinimum, a.NotificationMinimum)
}

func TestSetTrackErrorCountsToTrue(t *testing.T) {
	a := new(AdminNotificationManager)
	funcOpt := SetTrackErrorCountsToTrue(a)
	funcOpt(a)
	assert.True(t, a.TrackErrorCounts)
}

func TestSetDatabaseReadOnlyToTrue(t *testing.T) {
	a := new(AdminNotificationManager)
	funcOpt := SetDatabaseReadOnlyToTrue(a)
	funcOpt(a)
	assert.True(t, a.DatabaseReadOnly)
}

// type AdminNotificationManager struct {
// 	DatabaseReadOnly bool
// 	notificationSourceWg sync.WaitGroup
// 	adminErrorChan chan Notification
// 	allServiceCounts map[string]*serviceErrorCounts
// }

// AdminNotificationManagerOption is a functional option that should be used as an argument to NewAdminNotificationManager to set various fields
// of the AdminNotificationManager
// For example:
//
//	 f := func(a *AdminNotificationManager) error {
//		  a.NotificationMinimum = 42
//	   return nil
//	 }
//	 g := func(a *AdminNotificationManager) error {
//		  a.DatabaseReadOnly = false
//	   return nil
//	 }
//	 manager := NewAdminNotificationManager(context.Background, f, g)
// type AdminNotificationManagerOption func(*AdminNotificationManager) error