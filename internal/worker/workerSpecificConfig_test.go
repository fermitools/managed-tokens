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

package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetWorkerRetryValueFromConfig(t *testing.T) {
	c := &Config{}
	c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
	c.workerSpecificConfig[GetKerberosTicketsWorkerType] = make(map[WorkerSpecificConfigOption]any, 0)
	c.workerSpecificConfig[GetKerberosTicketsWorkerType][NumRetriesOption] = uint(5)

	// Test case: Worker type exists in the config
	val, err := getWorkerNumRetriesValueFromConfig(*c, GetKerberosTicketsWorkerType)
	assert.Nil(t, err)
	assert.Equal(t, uint(5), val)

	// Test case: Worker type does not exist in the config
	val, err = getWorkerNumRetriesValueFromConfig(*c, PushTokensWorkerType)
	assert.NotNil(t, err)
	assert.Equal(t, uint(0), val)

	// Test case: Worker type exists in the config but value is not of type uint
	c.workerSpecificConfig[GetKerberosTicketsWorkerType][NumRetriesOption] = "invalid"
	val, err = getWorkerNumRetriesValueFromConfig(*c, GetKerberosTicketsWorkerType)
	assert.NotNil(t, err)
	assert.Equal(t, uint(0), val)
}
func TestIsValidWorkerSpecificConfigOption(t *testing.T) {
	// Test case: Valid worker specific config option
	validOption := isValidWorkerSpecificConfigOption(NumRetriesOption)
	assert.True(t, validOption)

	// Test case: Invalid worker specific config option
	invalidOption := isValidWorkerSpecificConfigOption(WorkerSpecificConfigOption(2))
	assert.False(t, invalidOption)
}

func TestGetWorkerRetrySleepValueFromConfig(t *testing.T) {
	c := &Config{}
	c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
	c.workerSpecificConfig[GetKerberosTicketsWorkerType] = make(map[WorkerSpecificConfigOption]any, 0)
	c.workerSpecificConfig[GetKerberosTicketsWorkerType][RetrySleepOption] = 5 * time.Second

	// Test case: Worker type exists in the config
	val, err := getWorkerRetrySleepValueFromConfig(*c, GetKerberosTicketsWorkerType)
	assert.Nil(t, err)
	assert.Equal(t, 5*time.Second, val)

	// Test case: Worker type does not exist in the config
	val, err = getWorkerRetrySleepValueFromConfig(*c, PushTokensWorkerType)
	assert.NotNil(t, err)
	assert.Equal(t, time.Duration(0), val)

	// Test case: Worker type exists in the config but value is not of type time.Duration
	c.workerSpecificConfig[GetKerberosTicketsWorkerType][RetrySleepOption] = "invalid"
	val, err = getWorkerRetrySleepValueFromConfig(*c, GetKerberosTicketsWorkerType)
	assert.NotNil(t, err)
	assert.Equal(t, time.Duration(0), val)
}

func TestSetWorkerSpecificConfigOption(t *testing.T) {
	type testCase struct {
		description    string
		workerType     WorkerType
		option         WorkerSpecificConfigOption
		value          any
		expectedConfig *Config
		expectedErr    error
	}

	testCases := []testCase{
		{
			description: "Set NumRetries option",
			workerType:  GetKerberosTicketsWorkerType,
			option:      NumRetriesOption,
			value:       5,
			expectedConfig: &Config{
				workerSpecificConfig: map[WorkerType]map[WorkerSpecificConfigOption]any{
					GetKerberosTicketsWorkerType: {
						NumRetriesOption: 5,
					},
				},
			},
			expectedErr: nil,
		},
		{
			description: "Set RetrySleep option",
			workerType:  GetKerberosTicketsWorkerType,
			option:      RetrySleepOption,
			value:       5 * time.Second,
			expectedConfig: &Config{
				workerSpecificConfig: map[WorkerType]map[WorkerSpecificConfigOption]any{
					GetKerberosTicketsWorkerType: {
						RetrySleepOption: 5 * time.Second,
					},
				},
			},
			expectedErr: nil,
		},
		{
			description:    "Invalid worker type",
			workerType:     invalidWorkerType,
			option:         NumRetriesOption,
			value:          5,
			expectedConfig: nil,
			expectedErr:    errors.New("invalid worker type"),
		},
		{
			description:    "Invalid worker specific config option",
			workerType:     GetKerberosTicketsWorkerType,
			option:         invalidWorkerSpecificConfigOption,
			value:          5,
			expectedConfig: nil,
			expectedErr:    errors.New("invalid worker-specific configuration option"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			err := SetWorkerSpecificConfigOption(tc.workerType, tc.option, tc.value)(c)
			if tc.expectedErr == nil {
				assert.Nil(t, err)
				assert.Equal(t, *tc.expectedConfig, *c)
				return
			}
			// Non-nil error, so make sure our error message is correct
			assert.ErrorContains(t, err, tc.expectedErr.Error())
		})
	}
}
