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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fermitools/managed-tokens/internal/service"
)

func TestGetDefaultRoleFileDestinationTemplateValueFromExtras(t *testing.T) {
	testService := service.NewService("test_service")

	type testCase struct {
		description      string
		setKeyValFunc    func(*Config) error
		expectedTemplate string
		expectedOk       bool
	}

	testCases := []testCase{
		{
			"Nothing set",
			func(c *Config) error { return nil },
			"",
			false,
		},
		{
			"Valid setting",
			SetSupportedExtrasKeyValue(DefaultRoleFileDestinationTemplate, "foobar"),
			"foobar",
			true,
		},
		{
			"Invalid setting",
			SetSupportedExtrasKeyValue(DefaultRoleFileDestinationTemplate, 12345),
			"",
			false,
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				config, _ := NewConfig(testService, test.setKeyValFunc)
				val, ok := GetDefaultRoleFileDestinationTemplateValueFromExtras(config)
				assert.Equal(t, test.expectedTemplate, val)
				assert.Equal(t, test.expectedOk, ok)
			},
		)
	}
}

func TestGetFileCopierOptionsFromExtras(t *testing.T) {
	testService := service.NewService("test_service")

	type testCase struct {
		description   string
		setKeyValFunc func(*Config) error
		expectedOpts  []string
		expectedOk    bool
	}

	testCases := []testCase{
		{
			"Default case",
			func(c *Config) error { return nil },
			defaultFileCopierOpts,
			true,
		},
		{
			"Valid opts stored",
			SetSupportedExtrasKeyValue(FileCopierOptions, []string{"thisisvalid", "--opts"}),
			[]string{"thisisvalid", "--opts"},
			true,
		},
		{
			"Invalid opts stored - wrong type",
			SetSupportedExtrasKeyValue(FileCopierOptions, 12345),
			nil,
			false,
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				config, _ := NewConfig(testService, test.setKeyValFunc)
				val, ok := GetFileCopierOptionsFromExtras(config)
				assert.Equal(t, test.expectedOpts, val)
				assert.Equal(t, test.expectedOk, ok)
			},
		)
	}
}

func TestGetSSHOptionsFromExtras(t *testing.T) {

	type testCase struct {
		description  string
		setupFunc    func() *Config
		expectedOpts []string
		expectedOk   bool
	}

	testCases := []testCase{
		{
			"No options stored",
			func() *Config { return &Config{} },
			[]string{},
			true,
		},
		{
			"Valid opts",
			func() *Config {
				c := new(Config)
				c.Extras = make(map[supportedExtrasKey]any)
				c.Extras[SSHOptions] = []string{"foo", "bar"}
				return c
			},
			[]string{"foo", "bar"},
			true,
		},
		{
			"Invalid opts",
			func() *Config {
				c := new(Config)
				c.Extras = make(map[supportedExtrasKey]any)
				c.Extras[SSHOptions] = "thisisastring.  Oops"
				return c
			},
			nil,
			false,
		},
	}

	// Wrong type
	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				c := test.setupFunc()
				opts, ok := GetSSHOptionsFromExtras(c)
				assert.Equal(t, test.expectedOpts, opts)
				assert.Equal(t, test.expectedOk, ok)
			},
		)
	}
}
