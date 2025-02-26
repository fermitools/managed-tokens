package main

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/fermitools/managed-tokens/internal/db"
)

func TestGetDesiredUIDByOverrideOrLookup(t *testing.T) {
	serviceConfigPath = "service_path"
	tempDir := t.TempDir()

	type testCase struct {
		description       string
		viperSetupFunc    func()
		databaseSetupFunc func() *db.ManagedTokensDatabase
		expectedUID       uint32
		expectedError     error
	}

	testCases := []testCase{
		{
			description: "Valid uid override",
			viperSetupFunc: func() {
				viper.Set(serviceConfigPath+".desiredUIDOverride", 123)
			},
			databaseSetupFunc: func() *db.ManagedTokensDatabase { return nil },
			expectedUID:       123,
			expectedError:     nil,
		},
		{
			description:       "No valid database",
			viperSetupFunc:    func() {},
			databaseSetupFunc: func() *db.ManagedTokensDatabase { return nil },
			expectedUID:       0,
			expectedError:     errors.New("no valid database to read UID from"),
		},
		{
			description: "Get UID from DB, error",
			viperSetupFunc: func() {
				viper.Set(serviceConfigPath+".account", "testaccount")
			},
			databaseSetupFunc: func() *db.ManagedTokensDatabase {
				filename := tempDir + "/test_nodata.db"
				d, err := db.OpenOrCreateDatabase(filename)
				if err != nil {
					panic("Could not open test database" + err.Error())
				}
				return d
			},
			expectedUID:   0,
			expectedError: errors.New("could not get UID by username: sql: no rows in result set"),
		},
		{
			description: "Get UID from DB, success",
			viperSetupFunc: func() {
				viper.Set(serviceConfigPath+".account", "testaccount")
			},
			databaseSetupFunc: func() *db.ManagedTokensDatabase {
				filename := tempDir + "/test_populated.db"
				d, err := db.OpenOrCreateDatabase(filename)
				if err != nil {
					panic("Could not open test database" + err.Error())
				}
				f := fakeFerryUIDDatum{"testaccount", 123}
				err = d.InsertUidsIntoTableFromFERRY(context.Background(), []db.FerryUIDDatum{f})
				if err != nil {
					panic("Could not insert test data into database" + err.Error())
				}
				return d
			},
			expectedUID:   123,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			defer viper.Reset()
			tc.viperSetupFunc()
			uid, err := getDesiredUIDByOverrideOrLookup(context.Background(), serviceConfigPath, tc.databaseSetupFunc())

			assert.Equal(t, tc.expectedUID, uid)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
		})
	}
}

type fakeFerryUIDDatum struct {
	username string
	uid      int
}

func (f fakeFerryUIDDatum) Username() string {
	return f.username
}
func (f fakeFerryUIDDatum) Uid() int {
	return f.uid
}
func (f fakeFerryUIDDatum) String() string {
	return "NOT IMPLEMENTED"
}
