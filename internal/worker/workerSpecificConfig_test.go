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
	t.Run("Valid case", func(t *testing.T) {
		c := &Config{}
		c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
		c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
		c.workerSpecificConfig[PushTokens][NumRetriesOption] = uint(5)
		val, err := getWorkerNumRetriesValueFromConfig(*c, PushTokens)
		assert.Nil(t, err)
		assert.Equal(t, uint(5), val)
	})

	// Specific error cases
	type testCase struct {
		description    string
		setupFunc      func(c *Config)
		testWorkerType WorkerType
		errContains    string
	}

	testCases := []testCase{
		{
			description:    "Empty config",
			setupFunc:      func(c *Config) {},
			testWorkerType: PushTokens,
			errContains:    "given worker type not found in worker Config",
		},
		{
			description: "Worker type does not exist in the config",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
			},
			testWorkerType: PushTokens,
			errContains:    "given worker-specific configuration option not set in worker Config",
		},
		{
			description: "Worker type is invalid",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
			},
			testWorkerType: WorkerType(invalid),
			errContains:    "invalid worker type",
		},
		{
			description: "Value is not of type time.Duration",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
				c.workerSpecificConfig[PushTokens][NumRetriesOption] = "invalid"
			},
			testWorkerType: PushTokens,
			errContains:    "is not of type uint",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setupFunc(c)
			val, err := getWorkerNumRetriesValueFromConfig(*c, tc.testWorkerType)
			assert.ErrorContains(t, err, tc.errContains)
			assert.Equal(t, uint(0), val)
		})
	}
}
func TestIsValidWorkerSpecificConfigOption(t *testing.T) {
	// Test case: Valid worker specific config option
	validOption := isValidWorkerSpecificConfigOption(NumRetriesOption)
	assert.True(t, validOption)

	// Test case: Invalid worker specific config option
	invalidOption := isValidWorkerSpecificConfigOption(WorkerSpecificConfigOption(invalidWorkerSpecificConfigOption))
	assert.False(t, invalidOption)
}

func TestGetWorkerRetrySleepValueFromConfig(t *testing.T) {
	t.Run("Valid case", func(t *testing.T) {
		c := &Config{}
		c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
		c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
		c.workerSpecificConfig[PushTokens][RetrySleepOption] = 5 * time.Second
		val, err := getWorkerRetrySleepValueFromConfig(*c, PushTokens)
		assert.Nil(t, err)
		assert.Equal(t, 5*time.Second, val)
	})

	// Specific error cases
	type testCase struct {
		description    string
		setupFunc      func(c *Config)
		testWorkerType WorkerType
		errContains    string
	}

	testCases := []testCase{
		{
			description:    "Empty config",
			setupFunc:      func(c *Config) {},
			testWorkerType: PushTokens,
			errContains:    "given worker type not found in worker Config",
		},
		{
			description: "Worker type does not exist in the config",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
			},
			testWorkerType: PushTokens,
			errContains:    "given worker-specific configuration option not set in worker Config",
		},
		{
			description: "Worker type is invalid",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
			},
			testWorkerType: WorkerType(invalid),
			errContains:    "invalid worker type",
		},
		{
			description: "Value is not of type time.Duration",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[PushTokens] = make(map[WorkerSpecificConfigOption]any, 0)
				c.workerSpecificConfig[PushTokens][RetrySleepOption] = "invalid"
			},
			testWorkerType: PushTokens,
			errContains:    "is not of type time.Duration",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setupFunc(c)
			val, err := getWorkerRetrySleepValueFromConfig(*c, tc.testWorkerType)
			assert.ErrorContains(t, err, tc.errContains)
			assert.Equal(t, time.Duration(0), val)
		})
	}

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
			workerType:  GetKerberosTickets,
			option:      NumRetriesOption,
			value:       5,
			expectedConfig: &Config{
				workerSpecificConfig: map[WorkerType]map[WorkerSpecificConfigOption]any{
					GetKerberosTickets: {
						NumRetriesOption: 5,
					},
				},
			},
			expectedErr: nil,
		},
		{
			description: "Set RetrySleep option",
			workerType:  GetKerberosTickets,
			option:      RetrySleepOption,
			value:       5 * time.Second,
			expectedConfig: &Config{
				workerSpecificConfig: map[WorkerType]map[WorkerSpecificConfigOption]any{
					GetKerberosTickets: {
						RetrySleepOption: 5 * time.Second,
					},
				},
			},
			expectedErr: nil,
		},
		{
			description:    "Invalid worker type",
			workerType:     invalid,
			option:         NumRetriesOption,
			value:          5,
			expectedConfig: nil,
			expectedErr:    errors.New("invalid worker type"),
		},
		{
			description:    "Invalid worker specific config option",
			workerType:     GetKerberosTickets,
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

func TestGetAlternateTokenGetterOptionFromConfig(t *testing.T) {
	validWorkerType := GetToken
	invalidWorkerType := WorkerType(invalid)
	mockGetter := &fakeTokenGetter{}

	type testCase struct {
		description string
		setupFunc   func(c *Config)
		workerType  WorkerType
		expectedVal any
		expectedErr string
	}

	testCases := []testCase{
		{
			description: "Valid case",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][AlternateTokenGetterOption] = mockGetter
			},
			workerType:  validWorkerType,
			expectedVal: mockGetter,
			expectedErr: "",
		},
		{
			description: "No worker type map in config",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			workerType:  validWorkerType,
			expectedVal: nil,
			expectedErr: "given worker type not found in worker Config",
		},
		{
			description: "No AlternateTokenGetterOption found",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
			},
			workerType:  validWorkerType,
			expectedVal: nil,
			expectedErr: "given worker-specific configuration option not set in worker Config",
		},
		{
			description: "Value is not of type TokenGetter",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][AlternateTokenGetterOption] = "not-a-token-getter"
			},
			workerType:  validWorkerType,
			expectedVal: nil,
			expectedErr: "is not of type TokenGetter",
		},
		{
			description: "Invalid worker type",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			workerType:  invalidWorkerType,
			expectedVal: nil,
			expectedErr: "invalid worker type",
		},
		{
			description: "Worker type not in valid list",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				unsupportedWorkerType := PushTokens
				c.workerSpecificConfig[unsupportedWorkerType] = make(map[WorkerSpecificConfigOption]any)
			},
			workerType:  PushTokens,
			expectedVal: nil,
			expectedErr: "is not in the list of valid worker types",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setupFunc(c)
			val, err := getAlternateTokenGetterOptionFromConfig(*c, tc.workerType)
			if tc.expectedErr != "" {
				assert.Nil(t, val)
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedVal, val)
		})
	}
}

func TestGetAlternateTokenStorerAndGetterOptionFromConfig(t *testing.T) {
	validWorkerType := StoreAndGetToken
	invalidWorkerType := WorkerType(invalid)
	mockStorerAndGetter := &fakeTokenStorerAndGetter{}

	type testCase struct {
		description string
		setupFunc   func(c *Config)
		workerType  WorkerType
		expectedVal any
		expectedErr string
	}

	testCases := []testCase{
		{
			description: "Valid case",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][AlternateTokenStorerAndGetterOption] = mockStorerAndGetter
			},
			workerType:  validWorkerType,
			expectedVal: mockStorerAndGetter,
			expectedErr: "",
		},
		{
			description: "Value is not of type TokenStorerAndGetter",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][AlternateTokenStorerAndGetterOption] = "not-a-token-storer-and-getter"
			},
			workerType:  validWorkerType,
			expectedVal: nil,
			expectedErr: "is not of type TokenStorerAndGetter",
		},
		{
			description: "Invalid worker type",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			workerType:  invalidWorkerType,
			expectedVal: nil,
			expectedErr: "invalid worker type",
		},
		{
			description: "Worker type not in valid list",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				unsupportedWorkerType := PushTokens
				c.workerSpecificConfig[unsupportedWorkerType] = make(map[WorkerSpecificConfigOption]any)
			},
			workerType:  PushTokens,
			expectedVal: nil,
			expectedErr: "is not in the list of valid worker types",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setupFunc(c)
			val, err := getAlternateTokenStorerAndGetterOptionFromConfig(*c, tc.workerType)
			if tc.expectedErr != "" {
				assert.Nil(t, val)
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedVal, val)
		})
	}

	// Test Cases with specific errors
	type testCase2 struct {
		description string
		setup       func(c *Config)
		expectedErr error
	}

	testCases2 := []testCase2{
		{
			description: "No worker type map in config",
			setup: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			expectedErr: errNoWorkerTypeMapInConfig,
		},
		{
			description: "No AlternateTokenStorerAndGetterOption found",
			setup: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[StoreAndGetToken] = make(map[WorkerSpecificConfigOption]any)
			},
			expectedErr: errOptionNotSetInConfig,
		},
	}

	for _, tc := range testCases2 {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setup(c)
			val, err := getAlternateTokenStorerAndGetterOptionFromConfig(*c, validWorkerType)
			assert.Nil(t, val)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestGetCachedTokenStorerOptionFromConfig(t *testing.T) {
	validWorkerType := StoreAndGetToken
	invalidWorkerType := WorkerType(invalid)

	type testCase struct {
		description string
		setupFunc   func(c *Config)
		workerType  WorkerType
		expectedVal bool
		expectedErr string
	}

	testCases := []testCase{
		{
			description: "Valid case",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][CachedTokenStorerOption] = true
			},
			workerType:  validWorkerType,
			expectedVal: true,
			expectedErr: "",
		},
		{
			description: "Value is not of type bool",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType] = make(map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[validWorkerType][CachedTokenStorerOption] = "not-a-bool"
			},
			workerType:  validWorkerType,
			expectedVal: false,
			expectedErr: "is not of type bool",
		},
		{
			description: "Invalid worker type",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			workerType:  invalidWorkerType,
			expectedVal: false,
			expectedErr: "invalid worker type",
		},
		{
			description: "Worker type not in valid list",
			setupFunc: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				unsupportedWorkerType := PushTokens
				c.workerSpecificConfig[unsupportedWorkerType] = make(map[WorkerSpecificConfigOption]any)
			},
			workerType:  PushTokens,
			expectedVal: false,
			expectedErr: "is not in the list of valid worker types",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setupFunc(c)
			val, err := getCachedTokenStorerOptionFromConfig(*c, tc.workerType)
			if tc.expectedErr != "" {
				assert.False(t, val)
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedVal, val)
		})
	}

	// Test cases with specific errors
	type testCase2 struct {
		description string
		setup       func(c *Config)
		expectedErr error
	}

	testCases2 := []testCase2{
		{
			description: "No worker type map in config",
			setup: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
			},
			expectedErr: errNoWorkerTypeMapInConfig,
		},
		{
			description: "No CachedTokenStorerOption found",
			setup: func(c *Config) {
				c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
				c.workerSpecificConfig[StoreAndGetToken] = make(map[WorkerSpecificConfigOption]any)
			},
			expectedErr: errOptionNotSetInConfig,
		},
	}

	for _, tc := range testCases2 {
		t.Run(tc.description, func(t *testing.T) {
			c := &Config{}
			tc.setup(c)
			val, err := getCachedTokenStorerOptionFromConfig(*c, validWorkerType)
			assert.False(t, val)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestGetWorkerTypeMapFromConfig(t *testing.T) {
	// Good config
	c := &Config{}
	c.workerSpecificConfig = make(map[WorkerType]map[WorkerSpecificConfigOption]any)
	c.workerSpecificConfig[GetKerberosTickets] = make(map[WorkerSpecificConfigOption]any, 0)
	c.workerSpecificConfig[GetKerberosTickets][NumRetriesOption] = uint(5)

	validWorkerTypeList := []WorkerType{GetKerberosTickets, GetToken}

	// Specific error cases
	type testCase1 struct {
		description string
		config      Config
		workerType  WorkerType
		expectedErr error
	}

	testCases := []testCase1{
		{
			description: "Worker type not set in config",
			config:      *c,
			workerType:  GetToken,
			expectedErr: errNoWorkerTypeMapInConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			m, err := getWorkerTypeMapFromConfig(tc.config, tc.workerType, validWorkerTypeList)
			assert.ErrorIs(t, err, tc.expectedErr)
			assert.Nil(t, m)
		})
	}

	// Test cases with non-specific errors

	w := GetKerberosTickets
	type testCase2 struct {
		description         string
		workerType          WorkerType
		validWorkerTypes    []WorkerType
		expected            map[WorkerSpecificConfigOption]any
		expectedErrNil      bool
		expectedErrContains string
	}

	testCases2 := []testCase2{
		{
			description:      "Valid case",
			workerType:       w,
			validWorkerTypes: validWorkerTypeList,
			expected:         c.workerSpecificConfig[w],
			expectedErrNil:   true,
		},
		{
			description:      "Valid case - no restrictions on worker types",
			workerType:       w,
			validWorkerTypes: nil,
			expected:         c.workerSpecificConfig[w],
			expectedErrNil:   true,
		},
		{
			description:         "Invalid worker type",
			workerType:          WorkerType(255),
			validWorkerTypes:    nil,
			expected:            nil,
			expectedErrNil:      false,
			expectedErrContains: "invalid worker type",
		},
		{
			description:         "Worker type not in valid list",
			workerType:          PushTokens,
			validWorkerTypes:    validWorkerTypeList,
			expected:            nil,
			expectedErrNil:      false,
			expectedErrContains: "is not in the list of valid worker types",
		},
	}
	for _, tc := range testCases2 {
		t.Run(tc.description, func(t *testing.T) {
			m, err := getWorkerTypeMapFromConfig(*c, tc.workerType, tc.validWorkerTypes)
			if tc.expectedErrNil {
				assert.Nil(t, err)
				assert.Equal(t, tc.expected, m)
				return
			}
			assert.Nil(t, m)
			assert.ErrorContains(t, err, tc.expectedErrContains)
		})
	}
}
