package worker

import (
	"errors"
	"testing"

	"github.com/shreyb/managed-tokens/internal/service"
)

// TestSetDefaultRoleFileTemplateValueInExtras ensures that SetDefaultRoleFileTemplateValueInExtras corrently
// sets the Config.Extras["DefaultRoleFileTemplate"] value when called
func TestSetDefaultRoleFileTemplateValueInExtras(t *testing.T) {
	s := service.NewService("testservice")
	testTemplateString := "testtemplate"
	c, _ := NewConfig(
		s,
		func(c *Config) error {
			SetDefaultRoleFileTemplateValueInExtras(c, testTemplateString)
			return nil
		},
	)
	if result := c.Extras["DefaultRoleFileTemplate"]; result != testTemplateString {
		t.Errorf("Wrong template string stored.  Expected %s, got %s", testTemplateString, result)
	}
}

// TestGetDefaultRoleFileTemplateValueFromExtras checks that GetDefaultRoleFileTemplateValueFromExtras properly type-checks and retrieves
// the stored value in the Config.Extras["DefaultRoleFileTemplate"] map
func TestGetDefaultRoleFileTemplateValueFromExtras(t *testing.T) {
	s := service.NewService("testservice")

	type testCase struct {
		stored        any
		expectedValue string
		expectedCheck bool
	}
	testCases := []testCase{
		{
			stored:        "testvalue",
			expectedValue: "testvalue",
			expectedCheck: true,
		},
		{
			stored:        5,
			expectedValue: "",
			expectedCheck: false,
		},
	}

	for _, test := range testCases {
		c, _ := NewConfig(s)
		c.Extras["DefaultRoleFileTemplate"] = test.stored
		result, check := GetDefaultRoleFileTemplateValueFromExtras(c)
		if check != test.expectedCheck {
			t.Errorf("Type assertion failed.  Expected type assertion check to return %t, got %t", test.expectedCheck, check)
		}
		if result != test.expectedValue {
			t.Errorf("Got wrong value from config.  Expected %s, got %s", test.expectedValue, result)
		}
	}
}

func TestParseDefaultRoleFileTemplateFromConfig(t *testing.T) {
	s := service.NewService("testservice")
	c, _ := NewConfig(s)
	c.DesiredUID = 12345

	type testCase struct {
		stored   any
		expected string
		err      error
	}

	goodTestCases := []testCase{
		{
			// Good case
			stored:   "/tmp/thisisagoodcase_{{.DesiredUID}}_{{.Experiment}}",
			expected: "/tmp/thisisagoodcase_12345_testservice",
			err:      nil,
		},
		{
			// Template string doesn't have any vars to fill - should be OK
			stored:   "thisshouldstillwork",
			expected: "thisshouldstillwork",
			err:      nil,
		},
		{
			// Wrong type - should produce error
			stored:   42,
			expected: "",
			err:      errors.New("error"),
		},
		{
			// This template should not execute - so we expect an error
			stored:   "thisshouldfailwithanexecerror{{.Doesntexist}}",
			expected: "",
			err:      errors.New("error"),
		},
	}

	for _, test := range goodTestCases {
		c.Extras["DefaultRoleFileTemplate"] = test.stored
		result, err := parseDefaultRoleFileTemplateFromConfig(c)
		if err == nil && test.err != nil {
			t.Errorf("Expected error of type %T, got nil instead", test.expected)
		}
		if result != test.expected {
			t.Errorf("Expected template string value %s, got %s instead", test.expected, result)
		}
	}
}