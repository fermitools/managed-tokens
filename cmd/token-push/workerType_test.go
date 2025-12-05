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

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/fermitools/managed-tokens/internal/worker"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetWorkerConfigStringSlice(t *testing.T) {
	viper.Reset()
	workerType := worker.GetKerberosTicketsWorkerType
	key := "myKey"
	expectedValue := []string{"value1", "value2"}

	// Set up the configuration
	viper.Set("workerType."+workerTypeToConfigString(workerType)+"."+key, expectedValue)

	// Call the function
	result := getWorkerConfigStringSlice(workerType, key)

	// Check the result
	assert.Equal(t, expectedValue, result)
}

func TestGetWorkerConfigInteger(t *testing.T) {
	workerType := worker.GetKerberosTicketsWorkerType
	key := "myKey"

	type testCase struct {
		description string
		testValue   any
		expected    any
	}

	testCases := []testCase{
		{
			description: "Valid int",
			testValue:   42,
			expected:    42,
		},
		{
			description: "Valid uint",
			testValue:   uint(42),
			expected:    uint(42),
		},
		{
			description: "Int that's typed as a float",
			testValue:   42.0,
			expected:    42,
		},
		{
			description: "Int that's typed as a float, but rounded down (very unlikely)",
			testValue:   41.99999999999999,
			expected:    42,
		},
		{
			description: "Float that's not close to an int",
			testValue:   42.5,
			expected:    0,
		},
		{
			description: "Not an int",
			testValue:   "invalidInteger",
			expected:    0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			defer viper.Reset()
			viper.Set("workerType."+workerTypeToConfigString(workerType)+"."+key, tc.testValue)
			var result any
			if _, ok := tc.testValue.(uint); ok {
				result = getWorkerConfigInteger[uint](workerType, key)
			} else {
				result = getWorkerConfigInteger[int](workerType, key)
			}
			assert.Equal(t, tc.expected, result)
		},
		)
	}
}

func TestGetWorkerConfigString(t *testing.T) {
	viper.Reset()
	workerType := worker.GetKerberosTicketsWorkerType
	key := "myKey"
	expectedValue := "myValue"

	// Set up the configuration
	viper.Set("workerType."+workerTypeToConfigString(workerType)+"."+key, expectedValue)

	// Call the function
	value := getWorkerConfigString(workerType, key)

	// Check the result
	if value != expectedValue {
		t.Errorf("Got wrong value for worker config string. Expected %s, got %s", expectedValue, value)
	}

	// Clean up the configuration
	viper.Reset()
}
func TestGetWorkerConfigValue(t *testing.T) {
	// Set up test cases
	testCases := []struct {
		worker.WorkerType
		key      string
		config   map[string]any
		expected any
	}{
		{
			WorkerType: worker.GetKerberosTicketsWorkerType,
			key:        "key1",
			config: map[string]any{
				"workerType.getKerberosTickets.key1": "value1",
			},
			expected: "value1",
		},
		{
			WorkerType: worker.GetKerberosTicketsWorkerType,
			key:        "key2",
			config: map[string]any{
				"workerType.getKerberosTickets.key2": 42,
			},
			expected: 42,
		},
		{
			WorkerType: worker.GetKerberosTicketsWorkerType,
			key:        "key3",
			config: map[string]any{
				"workerType.getKerberosTickets.key3": []string{"value1", "value2"},
			},
			expected: []string{"value1", "value2"},
		},
		{
			WorkerType: worker.GetKerberosTicketsWorkerType,
			key:        "key4",
			config:     map[string]any{},
			expected:   nil,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("WorkerType: %s, Key: %s", workerTypeToConfigString(tc.WorkerType), tc.key), func(t *testing.T) {
			// Set up test environment
			viper.Reset()
			for k, v := range tc.config {
				viper.Set(k, v)
			}

			// Call the function
			result := getWorkerConfigValue(tc.WorkerType, tc.key)

			// Check the result
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetWorkerConfigTimeDuration(t *testing.T) {
	workerType := worker.GetKerberosTicketsWorkerType
	key := "myKey"

	type testCase struct {
		description   string
		testValue     any
		expectedValue time.Duration
	}

	testCases := []testCase{
		{
			description:   "Valid duration string",
			testValue:     "5m",
			expectedValue: 5 * time.Minute,
		},
		{
			description:   "Invalid duration string",
			testValue:     "invalidDuration",
			expectedValue: time.Duration(0),
		},
		{
			description:   "Non-string value",
			testValue:     12345,
			expectedValue: time.Duration(0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Set up the configuration
			viper.Reset()
			defer viper.Reset()
			viper.Set("workerType."+workerTypeToConfigString(workerType)+"."+key, tc.testValue)

			result := getWorkerConfigTimeDuration(workerType, key)
			assert.Equal(t, tc.expectedValue, result)
		})
	}
}

func TestWorkerTypeToConfigString(t *testing.T) {
	tests := []struct {
		workerType worker.WorkerType
		expected   string
	}{
		{
			workerType: worker.GetKerberosTicketsWorkerType,
			expected:   "getKerberosTickets",
		},
		{
			workerType: worker.GetTokenWorkerType,
			expected:   "getToken",
		},
		{
			workerType: worker.StoreAndGetTokenWorkerType,
			expected:   "storeAndGetToken",
		},
		{
			workerType: worker.StoreAndGetTokenInteractiveWorkerType,
			expected:   "storeAndGetTokenInteractive",
		},
		{
			workerType: worker.PingAggregatorWorkerType,
			expected:   "pingAggregator",
		},
		{
			workerType: worker.PushTokensWorkerType,
			expected:   "pushTokens",
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("WorkerType: %s", test.workerType.String()), func(t *testing.T) {
			result := workerTypeToConfigString(test.workerType)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestWorkerTypeFromConfig(t *testing.T) {
	tests := []struct {
		description string
		input       string
		expectedWT  worker.WorkerType
		expectedOk  bool
	}{
		{
			description: "Valid config string for GetKerberosTicketsWorkerType",
			input:       "getKerberosTickets",
			expectedWT:  worker.GetKerberosTicketsWorkerType,
			expectedOk:  true,
		},
		{
			description: "Valid config string for GetTokenWorkerType",
			input:       "getToken",
			expectedWT:  worker.GetTokenWorkerType,
			expectedOk:  true,
		},
		{
			description: "Valid config string for StoreAndGetTokenWorkerType",
			input:       "storeAndGetToken",
			expectedWT:  worker.StoreAndGetTokenWorkerType,
			expectedOk:  true,
		},
		{
			description: "Valid config string for StoreAndGetTokenInteractiveWorkerType",
			input:       "storeAndGetTokenInteractive",
			expectedWT:  worker.StoreAndGetTokenInteractiveWorkerType,
			expectedOk:  true,
		},
		{
			description: "Valid config string for PingAggregatorWorkerType",
			input:       "pingAggregator",
			expectedWT:  worker.PingAggregatorWorkerType,
			expectedOk:  true,
		},
		{
			description: "Valid config string for PushTokensWorkerType",
			input:       "pushTokens",
			expectedWT:  worker.PushTokensWorkerType,
			expectedOk:  true,
		},
		{
			description: "Invalid config string",
			input:       "invalidWorkerType",
			expectedWT:  0,
			expectedOk:  false,
		},
		{
			description: "Empty string",
			input:       "",
			expectedWT:  0,
			expectedOk:  false,
		},
		{
			description: "Wrong case",
			input:       "GetKerberosTickets",
			expectedWT:  0,
			expectedOk:  false,
		},
		{
			description: "Gibberish string",
			input:       "asdlkjasdklj",
			expectedWT:  0,
			expectedOk:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			wt, ok := workerTypeFromConfig(tc.input)
			assert.Equal(t, tc.expectedWT, wt)
			assert.Equal(t, tc.expectedOk, ok)
		})
	}
}
