package vaultToken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"slices"
	"testing"

	"github.com/fermitools/managed-tokens/internal/environment"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultBearerTokenFileLocation(t *testing.T) {
	// getDefaultBearerTokenFileLocation returns the default location for the bearer token file, following the logic of the WLCG Bearer Token Discovery specification:
	// 1. If the BEARER_TOKEN_FILE environment variable is set, use that
	// 2. If the XDG_RUNTIME_DIR environment variable is set, use $XDG_RUNTIME_DIR/bt_u<uid>
	// 3. Otherwise, use /tmp/bt_u<uid>
	//
	// See https://zenodo.org/records/3937438 for more details
	currentUser, _ := user.Current()
	uid := currentUser.Uid

	type testCase struct {
		description            string
		bearerTokenFileSetFunc func(*testing.T)
		xdgRuntimeDirSetFunc   func(*testing.T) (cleanup func())
		expectedResult         string
		expectedErrNil         bool
	}
	testCases := []testCase{
		{
			"BEARER_TOKEN_FILE is set",
			func(t *testing.T) { t.Setenv("BEARER_TOKEN_FILE", "/path/to/bearer_token_file") },
			func(t *testing.T) func() { return nil },
			"/path/to/bearer_token_file",
			true,
		},
		{
			"XDG_RUNTIME_DIR is set, BEARER_TOKEN_FILE is not set",
			func(t *testing.T) {},
			func(t *testing.T) func() {
				t.Setenv("XDG_RUNTIME_DIR", "/fake/xdg/path/1000")
				return nil
			},
			fmt.Sprintf("/fake/xdg/path/1000/bt_u%s", uid),
			true,
		},
		{
			"BEARER_TOKEN_FILE is set, XDG_RUNTIME_DIR is also set",
			func(t *testing.T) { t.Setenv("BEARER_TOKEN_FILE", "/path/to/bearer_token_file") },
			func(t *testing.T) func() {
				t.Setenv("XDG_RUNTIME_DIR", "/fake/xdg/path/1000")
				return nil
			},
			"/path/to/bearer_token_file",
			true,
		},
		{
			"Neither BEARER_TOKEN_FILE nor XDG_RUNTIME_DIR is set",
			func(t *testing.T) {},
			func(t *testing.T) func() {
				oldValue, existed := os.LookupEnv("XDG_RUNTIME_DIR")
				if existed {
					os.Unsetenv("XDG_RUNTIME_DIR")
				}
				return func() {
					if existed {
						os.Setenv("XDG_RUNTIME_DIR", oldValue)
					}
				}
			},
			fmt.Sprintf("/tmp/bt_u%s", uid),
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				tc.bearerTokenFileSetFunc(t)
				cleanup := tc.xdgRuntimeDirSetFunc(t)
				if cleanup != nil {
					t.Cleanup(cleanup)
				}

				result, err := getDefaultBearerTokenFileLocation()
				if !tc.expectedErrNil {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				assert.Equal(t, tc.expectedResult, result)
			},
		)
	}
}

func createFileIfNotExist(path string) string {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		os.Create(path)
	}
	return path
}

func TestNewHtgettokenClient(t *testing.T) {
	type expectedError struct {
		isNotNil bool
		contains string
	}

	env := &environment.CommandEnvironment{}
	opts := []string{"--test-option1", "--test-option2", "value2"}
	defaultVaultTokenFile, _ := getDefaultVaultTokenLocation()
	defaultOutfile, _ := getDefaultBearerTokenFileLocation()

	type testCase struct {
		description    string
		vaultServer    string
		vaultTokenFile string
		outFile        string
		expectedResult *HtgettokenClient
		expectedErr    expectedError
	}

	testCases := []testCase{
		{
			"All parameters provided",
			"https://vault.example.com",
			"/path/to/vault_token_file",
			"/path/to/output_file",
			&HtgettokenClient{
				vaultServer:        "https://vault.example.com",
				vaultTokenFile:     "/path/to/vault_token_file",
				outFile:            "/path/to/output_file",
				options:            opts,
				CommandEnvironment: env,
			},
			expectedError{},
		},
		{
			"Missing vault server",
			"",
			"/path/to/vault_token_file",
			"/path/to/output_file",
			nil,
			expectedError{
				isNotNil: true,
				contains: "vault server cannot be empty",
			},
		},
		{
			"Missing vaultTokenFile",
			"https://vault.example.com",
			"",
			"/path/to/output_file",
			&HtgettokenClient{
				vaultServer:        "https://vault.example.com",
				vaultTokenFile:     defaultVaultTokenFile,
				outFile:            "/path/to/output_file",
				options:            opts,
				CommandEnvironment: env,
			},
			expectedError{},
		},
		{
			"Missing outFile",
			"https://vault.example.com",
			"/path/to/vault_token_file",
			"",
			&HtgettokenClient{
				vaultServer:        "https://vault.example.com",
				vaultTokenFile:     "/path/to/vault_token_file",
				outFile:            defaultOutfile,
				options:            opts,
				CommandEnvironment: env,
			},
			expectedError{},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				client, err := NewHtgettokenClient(tc.vaultServer, tc.vaultTokenFile, tc.outFile, env, opts...)
				if !tc.expectedErr.isNotNil {
					assert.Equal(t, tc.expectedResult.vaultServer, client.vaultServer)
					assert.Equal(t, tc.expectedResult.vaultTokenFile, client.vaultTokenFile)
					assert.Equal(t, tc.expectedResult.outFile, client.outFile)
					assert.True(t, slices.Equal(tc.expectedResult.options, client.options))
					assert.Equal(t, *tc.expectedResult.CommandEnvironment, *client.CommandEnvironment)
				}

				assert.Equal(t, tc.expectedErr.isNotNil, err != nil)

				if tc.expectedErr.isNotNil {
					assert.Contains(t, err.Error(), tc.expectedErr.contains)
				}
			},
		)
	}

}

func TestHtgettokenClientWithVerbose(t *testing.T) {
	h := &HtgettokenClient{}
	h = h.WithVerbose()
	assert.True(t, h.verbose)
}

func TestHtgettokenClientPrepareCmdArgs(t *testing.T) {
	h := &HtgettokenClient{
		vaultServer:    "https://vault.example.com",
		vaultTokenFile: "/path/to/vault_token_file",
		outFile:        "/path/to/output_file",
		options: []string{
			"--option1",
			"--option2",
			"value2",
		},
	}

	issuer := "issuer_example"

	type testCase struct {
		description string
		role        string
		expected    []string
	}

	testCases := []testCase{
		{
			"With role",
			"role_example",
			[]string{
				"-a", h.vaultServer,
				"-i", issuer,
				"--vaulttokenfile", h.vaultTokenFile,
				"-o", h.outFile,
				"--option1",
				"--option2", "value2",
				"--role", "role_example",
			},
		},
		{
			"Without role",
			"",
			[]string{
				"-a", h.vaultServer,
				"-i", issuer,
				"--vaulttokenfile", h.vaultTokenFile,
				"-o", h.outFile,
				"--option1",
				"--option2", "value2",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				result := h.prepareCmdArgs(issuer, tc.role)
				assert.Equal(t, tc.expected, result)
			},
		)
	}
}

func TestHtgettokenClientGetToken(t *testing.T) {
	// Outfile to use for token tests
	outfilePath := path.Join(t.TempDir(), "htgettoken_test_token")

	// A bit of a hacky way to get our byte slice to check against
	goodBytes, cleanup := mockHtgettoken(t, outfilePath, 0)
	cleanup()

	type testCase struct {
		description             string
		interactive             bool
		cancelContext           bool
		setupMockHtgettokenFunc func(*testing.T) (cleanupFunc func())
		expectedToken           []byte
		expectedError           *errCheck
	}

	testCases := []testCase{
		{
			"Successful execution, non-interactive",
			false,
			false,
			func(t *testing.T) func() {
				_, cleanup := mockHtgettoken(t, outfilePath, 0)
				return cleanup
			},
			goodBytes,
			nil,
		},
		{
			"Context canceled before execution",
			false,
			true,
			func(t *testing.T) func() {
				_, cleanup := mockHtgettoken(t, outfilePath, 0)
				return cleanup
			},
			nil,
			&errCheck{contains: "context canceled before getting token"},
		},
		{
			"Execution error - interactive",
			true,
			false,
			func(t *testing.T) (cleanupFunc func()) {
				_, cleanup := mockHtgettoken(t, outfilePath, 1)
				return cleanup
			},
			nil,
			&errCheck{contains: "error running htgettoken to obtain bearer token"},
		},
		{
			"Execution error - non-interactive",
			false,
			false,
			func(t *testing.T) (cleanupFunc func()) {
				_, cleanup := mockHtgettoken(t, outfilePath, 2)
				return cleanup
			},
			nil,
			&errCheck{contains: "error running htgettoken to obtain bearer token"},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				cleanup := tc.setupMockHtgettokenFunc(t)
				defer cleanup()

				h, _ := NewHtgettokenClient(
					"https://vault.example.com",
					"/path/to/vault_token_file",
					outfilePath,
					&environment.CommandEnvironment{},
				)
				ctx, cancel := context.WithCancel(context.Background())
				if tc.cancelContext {
					cancel()
				}
				defer cancel()
				tokenBytes, err := h.GetToken(ctx, "issuer_example", "role_example", tc.interactive)
				assert.Equal(t, tc.expectedToken, tokenBytes)
				assert.True(t, tc.expectedError.containsErr(err))
				// Note: We don't care about the token check for this test, so the warnings will not cause test failures
			},
		)
	}
}

// Mock command that simulates successful htgettoken execution with exitCode given
func mockHtgettoken(t *testing.T, outfile string, exitCode uint) (tokenBytes []byte, cleanupFunc func()) {
	t.Helper()

	temp := t.TempDir()
	fakeHtgettokenPath := path.Join(temp, "htgettoken")
	scriptContent := fmt.Sprintf(`#!/bin/bash
echo "Fake htgettoken executed"
exit %d
`, exitCode)
	os.WriteFile(fakeHtgettokenPath, []byte(scriptContent), 0755)

	var fakeTokenBytes []byte = nil
	if exitCode == 0 {
		// Also create a fake bearer token file
		fakeTokenContent := "12345abcde"
		os.WriteFile(outfile, []byte(fakeTokenContent), 0644)
		fakeTokenBytes = []byte(fakeTokenContent)
	}

	oldPath, ok := vaultExecutables["htgettoken"]
	vaultExecutables["htgettoken"] = fakeHtgettokenPath

	cleanupFunc = func() {
		if ok {
			vaultExecutables["htgettoken"] = oldPath
		}
		if exitCode == 0 {
			os.Remove(outfile)
		}
	}
	return fakeTokenBytes, cleanupFunc
}

func TestCheckToken(t *testing.T) {
	// Note: we're missing one test case - where the establishing of a NewEnforcer doesn't work.

	// Fake token created from demo.scitokens.org that expires on 2222-02-22 22:22:22 UTC
	goodTokenString := "eyJhbGciOiJSUzI1NiIsImtpZCI6ImtleS1yczI1NiIsInR5cCI6IkpXVCJ9.eyJzY29wZSI6InJlYWQ6L3Byb3RlY3RlZCIsImF1ZCI6Imh0dHBzOi8vZGVtby5zY2l0b2tlbnMub3JnIiwidmVyIjoic2NpdG9rZW46Mi4wIiwiaXNzIjoiaHR0cHM6Ly9kZW1vLnNjaXRva2Vucy5vcmciLCJleHAiOjc5NTY5MTU3NDIsImlhdCI6MTc2NDAyNDM2OCwibmJmIjoxNzY0MDI0MzY4LCJ3bGNnLmdyb3VwcyI6WyIvZ3JvdXAiLCIvZ3JvdXAvcm9sZSJdLCJqdGkiOiI1OGQwN2EyZS1mYTg4LTQxMmUtOTA1My0wNmY2YjZhZDcyNzIifQ.dRpeZS3sQGb-rlqR27nlkTw0RzqxjKGErpUCSli0th02HvT1tfnlTvVxZX9mWTUQdZo3QnR5q83Yw7mLJtzLT-osqB1HQn98bWdsRZfe-cZzieBKbkkhevnskovO2jMQQeM6jGhtXaaLMSEJI9EGxM-yUPn7_WKWTsRKjbu-Snzg7KS8VqHnR0I-C_3PHPikPHLgq47C83kEewZ_thzi5wKYlP1NZVNaM5FG6P3Ul_KIHvKwenJ1aJOUrbRcervPALwKh5_vWvFW6ARrTR2Inv6lETHRIQtfsSSxImneRKHE4xUGU1Jfwrt54SZ-vDJcVbYSMq4ac18t_zQS_oAVLw"

	// Token created from demo.scitokens.org with wlcg.groups as a single string rather than a slice. Expires on 2222-02-22 22:22:22 UTC
	malformedGroupsTokenString := "eyJhbGciOiJSUzI1NiIsImtpZCI6ImtleS1yczI1NiIsInR5cCI6IkpXVCJ9.eyJzY29wZSI6InJlYWQ6L3Byb3RlY3RlZCIsImF1ZCI6Imh0dHBzOi8vZGVtby5zY2l0b2tlbnMub3JnIiwidmVyIjoic2NpdG9rZW46Mi4wIiwiaXNzIjoiaHR0cHM6Ly9kZW1vLnNjaXRva2Vucy5vcmciLCJleHAiOjc5NTY5MTU3NDIsImlhdCI6MTc2NDAyNTA5NiwibmJmIjoxNzY0MDI1MDk2LCJ3bGNnLmdyb3VwcyI6Ii9ncm91cCIsImp0aSI6IjU4ZDA3YTJlLWZhODgtNDEyZS05MDUzLTA2ZjZiNmFkNzI3MiJ9.A5wMXvX7dHBmr5tz8SXRmbFONCQ9kobEVxjMBKTgRMBcItqDlZi5dhHQgOf2hg6GuePpiou0d-8vmFTGwOiDhV2Lvj1x0W6103M7owgcdQ2_TuDMfso61F5cmNEdq13k-R-1Dq649zSypnOot0_FyIyGBudTTE1SkDK2KViwbalaLnBAof-CsqINPDNSDZU2Zxvz4U1yvDoaTnA1pcnqpg6xOLjnSMth4vNacrUjOhY_9pL83BbH7A7-DMukfIpR1r2QVzXsnpRsSRc1cIjmxPMNdeFDFAEh2njRS3JMkhAZ60KQA6UI9M-gHWrEoJjwq1giHrYQIV4IsmkEvYDuDQ"

	writeFakeToken := func(t *testing.T, tokenString string) func(t *testing.T) (tokenPath string) {
		return func(*testing.T) string {
			tokenFile := fmt.Sprintf("%s/fake_token.txt", t.TempDir())
			os.WriteFile(tokenFile, []byte(tokenString), 0644)
			return tokenFile
		}
	}

	type testCase struct {
		description    string
		setupTokenFile func(t *testing.T) string
		issuer         string
		role           string
		expectedError  *errCheck
	}

	testCases := []testCase{
		{
			"Valid token",
			writeFakeToken(t, goodTokenString),
			"group",
			"role",
			nil,
		},
		{
			"Token file does not exist",
			func(t *testing.T) string {
				return fmt.Sprintf("%s/non_existent_token.txt", t.TempDir())
			},
			"group",
			"role",
			&errCheck{contains: "no such file or directory"},
		},
		{
			"Invalid token content",
			writeFakeToken(t, "thisisnotavalidtoken"),
			"group",
			"role",
			&errCheck{contains: "failed to parse token"},
		},
		{
			"Token with invalid wlcg.groups claim",
			writeFakeToken(t, malformedGroupsTokenString),
			"group",
			"role",
			&errCheck{contains: "wlcg.groups claim"},
		},
		{
			"Wrong role in token",
			writeFakeToken(t, goodTokenString),
			"group",
			"wrongrole",
			&errCheck{contains: "token invalid"},
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.description,
			func(t *testing.T) {
				tokenFile := tc.setupTokenFile(t)
				err := checkToken(tokenFile, tc.issuer, tc.role)
				assert.True(t, tc.expectedError.containsErr(err))
			},
		)
	}
}
