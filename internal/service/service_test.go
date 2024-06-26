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

package service

import (
	"testing"
)

// TestNewService checks that NewService properly parses out the experiment and role from a passed in serviceName
func TestNewService(t *testing.T) {
	type testCase struct {
		description        string
		serviceName        string
		expectedExperiment string
		expectedRole       string
		expectedName       string
	}
	testCases := []testCase{
		{
			description:        "Experiment with no role (should assign default role)",
			serviceName:        "myawesomeexperiment",
			expectedExperiment: "myawesomeexperiment",
			expectedRole:       DefaultRole,
			expectedName:       "myawesomeexperiment",
		},
		{
			description:        "Experiment with role (should parse out experiment and role)",
			serviceName:        "myreallycoolexperiment_superrole",
			expectedExperiment: "myreallycoolexperiment",
			expectedRole:       "superrole",
			expectedName:       "myreallycoolexperiment_superrole",
		},
		{
			description:        "Malformed serviceName (should simply use serviceName as experiment, assign defualt role)",
			serviceName:        "weirdexperiment$@#@#!",
			expectedExperiment: "weirdexperiment$@#@#!",
			expectedRole:       DefaultRole,
			expectedName:       "weirdexperiment$@#@#!",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewService(tc.serviceName)
			if s.Experiment() != tc.expectedExperiment {
				t.Errorf("New service does not have the expected experiment name.  Wanted %s, got %s", tc.expectedExperiment, s.Experiment())
			}
			if s.Role() != tc.expectedRole {
				t.Errorf("New service does not have the expected role name.  Wanted %s, got %s", tc.expectedRole, s.Role())
			}
			if s.Name() != tc.expectedName {
				t.Errorf("New service does not have the expected service name.  Wanted %s, got %s", tc.expectedName, s.Name())
			}
		})
	}
}
