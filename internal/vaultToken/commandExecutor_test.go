package vaultToken

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// This test func covers all of the test cases for the interactiveExecutor and most of them for the nonInteractiveExecutor.
// The remaining non-interactive test cases are in TestNoninteractiveExecutorExecuteCommand
func TestExecutorExecuteCommand(t *testing.T) {
	// Find sh on the PATH
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Error("Couldn't find sh on PATH. These tests will fail")
	}

	type testCase struct {
		description           string
		testCommandAndContext func(t *testing.T) (*exec.Cmd, context.Context)
		err                   *errCheck
	}

	testCases := []testCase{
		{
			"Successful command execution",
			func(t *testing.T) (*exec.Cmd, context.Context) {
				ctx := context.Background()
				return exec.CommandContext(ctx, shPath, goodNoopCommand(t)), ctx
			},
			nil,
		},
		{
			"Context deadline exceeded before execution",
			func(t *testing.T) (*exec.Cmd, context.Context) {
				ctx2, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				t.Cleanup(cancel)
				time.Sleep(2 * time.Nanosecond)
				return exec.CommandContext(ctx2, shPath, goodNoopCommand(t)), ctx2
			},
			&errCheck{contains: "context deadline exceeded"},
		},
		{
			"Command execution error",
			func(t *testing.T) (*exec.Cmd, context.Context) {
				ctx := context.Background()
				return exec.CommandContext(ctx, shPath, badNoopCommand(t)), ctx
			},
			&errCheck{contains: "exit status 1"},
		},
	}

	exs := []commandExecutor{
		&interactiveExecutor{},
		&nonInteractiveExecutor{},
	}

	descStringAddOn := func(ex commandExecutor) string {
		switch ex.(type) {
		case *interactiveExecutor:
			return " interactive"
		case *nonInteractiveExecutor:
			return " non-interactive"
		default:
			return ""
		}
	}

	for _, tc := range testCases {
		for _, ex := range exs {
			t.Run(
				tc.description+descStringAddOn(ex),
				func(t *testing.T) {
					cmd, ctx := tc.testCommandAndContext(t)
					err := ex.executeCommand(ctx, cmd)
					assert.True(t, tc.err.containsErr(err))
				},
			)
		}
	}
}

func TestNoninteractiveExecutorExecuteCommand(t *testing.T) {
	// Find sh on the PATH
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Error("Couldn't find sh on PATH. These tests will fail")
	}

	type testCase struct {
		description           string
		testCommandAndContext func(t *testing.T) (*exec.Cmd, context.Context)
		err                   *errCheck
	}

	// This is a slice of testCases just in case we want to add more later
	testCases := []testCase{
		{
			"Auth needed error",
			func(t *testing.T) (*exec.Cmd, context.Context) {
				ctx := context.Background()
				// Create a command that exits with code 2 to simulate auth needed
				temp := t.TempDir()
				authNeededCommandPath := path.Join(temp, "authNeededCommand")
				os.WriteFile(authNeededCommandPath, []byte(`
#!/bin/bash
echo "Authentication needed for this command (test)"
exit 2
`), 0755)
				return exec.CommandContext(ctx, shPath, authNeededCommandPath), ctx

			},
			&errCheck{contains: "authentication needed"},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				cmd, ctx := tc.testCommandAndContext(t)
				executor := &nonInteractiveExecutor{}
				err := executor.executeCommand(ctx, cmd)
				assert.True(t, tc.err.containsErr(err))
			},
		)
	}
}

func TestCheckStdOutForErrorAuthNeeded(t *testing.T) {
	type testCase struct {
		description          string
		stdoutStderr         []byte
		expectedErrTypeCheck error
		expectedWrappedError error
	}

	testCases := []testCase{
		{
			"Random string - should not find result",
			[]byte("This is a random string"),
			nil,
			nil,
		},
		{
			"Auth needed",
			[]byte("Authentication needed for myservice"),
			&ErrAuthNeeded{},
			nil,
		},
		{
			"Auth needed - timeout",
			[]byte("Authentication needed for myservice\n\n\nblahblah\n\nhtgettoken: Polling for response took longer than 2 minutes"),
			&ErrAuthNeeded{},
			errHtgettokenTimeout,
		},
		{
			"Auth needed - permission denied",
			[]byte("Authentication needed for myservice\n\n\nblahblah\n\nhtgettoken: blahblah HTTP Error 403: Forbidden: permission denied"),
			&ErrAuthNeeded{},
			errHtgettokenPermissionDenied,
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				err := checkStdoutStderrForAuthNeededError(test.stdoutStderr)
				if err == nil && test.expectedErrTypeCheck == nil {
					return
				}
				var err1 *ErrAuthNeeded
				if !errors.As(err, &err1) {
					t.Errorf("Expected returned error to be of type *errAuthNeeded.  Got %T instead", err)
					return
				}

				if errVal := errors.Unwrap(err); !errors.Is(errVal, test.expectedWrappedError) {
					t.Errorf("Did not get expected wrapped error.  Expected error %v to be wrapped, but full error is %v.", test.expectedWrappedError, err)
				}

			},
		)
	}
}

// Set to nil if no error expected
type errCheck struct {
	contains string
}

func (e *errCheck) containsErr(err error) bool {
	// If e is nil, err must be nil as well
	if e == nil {
		return err == nil
	}
	// For non-nil e, err must be non-nil and contain the string
	return err != nil && strings.Contains(err.Error(), e.contains)
}

func goodNoopCommand(t *testing.T) string {
	t.Helper()
	temp := t.TempDir()
	goodCommandPath := path.Join(temp, "goodCommand")

	// Create a good command that just exits 0
	os.WriteFile(goodCommandPath, []byte(`
#!/bin/bash
exit 0
`), 0755)

	return goodCommandPath
}

func badNoopCommand(t *testing.T) string {
	t.Helper()
	temp := t.TempDir()
	badCommandPath := path.Join(temp, "goodCommand")

	// Create a bad command that just exits 0
	os.WriteFile(badCommandPath, []byte(`
#!/bin/bash
exit 1
`), 0755)

	return badCommandPath
}
