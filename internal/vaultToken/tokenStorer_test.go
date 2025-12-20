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

package vaultToken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/fermitools/managed-tokens/internal/environment"
)

func TestVaultStorerClientGetAndStoreToken(t *testing.T) {
	v := &VaultStorerClient{
		credd:              "mockCredd",
		vaultServer:        "mockVaultServer",
		CommandEnvironment: new(environment.CommandEnvironment),
	}
	serviceName := "test_service"

	type testCase struct {
		description            string
		ctx                    context.Context
		interactive            bool
		vaultStorerCommandMock func() (cleanup func(), err error)
		expectedErrNil         bool
		errContains            string
	}

	testCases := []testCase{
		{
			"Valid case",
			context.Background(),
			false,
			func() (func(), error) {
				temp := t.TempDir()
				oldPath := vaultExecutables["condor_vault_storer"]
				vaultExecutables["condor_vault_storer"] = path.Join(temp, "mock_condor_vault_storer_success.sh")
				cleanupFunc := func() {
					vaultExecutables["condor_vault_storer"] = oldPath
				}

				// Write the fake script
				scriptContent := `#!/bin/bash
exit 0
`
				if err := os.WriteFile(vaultExecutables["condor_vault_storer"], []byte(scriptContent), 0755); err != nil {
					return cleanupFunc, errors.New("Could not write mock condor_vault_storer script")
				}
				return cleanupFunc, nil
			},
			true,
			"",
		},
		{
			"Valid case, interactive",
			context.Background(),
			true,
			func() (func(), error) {
				temp := t.TempDir()
				oldPath := vaultExecutables["condor_vault_storer"]
				vaultExecutables["condor_vault_storer"] = path.Join(temp, "mock_condor_vault_storer_success.sh")
				cleanupFunc := func() {
					vaultExecutables["condor_vault_storer"] = oldPath
				}

				// Write the fake script
				scriptContent := `#!/bin/bash
exit 0
`
				if err := os.WriteFile(vaultExecutables["condor_vault_storer"], []byte(scriptContent), 0755); err != nil {
					return cleanupFunc, errors.New("Could not write mock condor_vault_storer script")
				}
				return cleanupFunc, nil
			},
			true,
			"",
		},
		{
			"Context already expired",
			func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			false,
			func() (func(), error) { return func() {}, nil },
			false,
			"context canceled",
		},
		{
			"Command fails",
			context.Background(),
			false,
			func() (func(), error) {
				temp := t.TempDir()
				oldPath := vaultExecutables["condor_vault_storer"]
				vaultExecutables["condor_vault_storer"] = path.Join(temp, "mock_condor_vault_storer_success.sh")
				cleanupFunc := func() {
					vaultExecutables["condor_vault_storer"] = oldPath
				}

				// Write the fake script
				scriptContent := `#!/bin/bash
exit 1
`
				if err := os.WriteFile(vaultExecutables["condor_vault_storer"], []byte(scriptContent), 0755); err != nil {
					return cleanupFunc, errors.New("Could not write mock condor_vault_storer script")
				}
				return cleanupFunc, nil
			},
			false,
			"error getting and storing vault token on credd",
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				cleanup, err := test.vaultStorerCommandMock()
				if err != nil {
					cleanup()
					t.Fatalf("Could not setup vault storer command mock: %s", err)
				}
				defer cleanup()
				err = v.GetAndStoreToken(test.ctx, serviceName, test.interactive)
				if test.expectedErrNil {
					if err != nil {
						t.Errorf("Expected nil error, got %s", err)
					}
					return // We expected err to be nil and it was, so we don't need to check any further
				}

				// Not expecting nil error
				if err == nil {
					t.Errorf("Expected non-nil error, got nil")
				}

				if !strings.Contains(err.Error(), test.errContains) {
					t.Errorf("Error does not contain expected string.  Expected to contain %s, got %s", test.errContains, err)
				}
			},
		)
	}

}

func TestVaultStorerClientSetupCmdWithEnvironment(t *testing.T) {
	v := &VaultStorerClient{
		credd:              "mockCredd",
		vaultServer:        "mockVaultServer",
		CommandEnvironment: new(environment.CommandEnvironment),
	}

	serviceName := "test_service"

	expected := exec.CommandContext(context.Background(), vaultExecutables["condor_vault_storer"], serviceName)
	result := v.setupCmdWithEnvironment(context.Background(), serviceName)

	if result.Path != expected.Path {
		t.Errorf("Got wrong executable to run.  Expected %s, got %s", expected.Path, result.Path)
	}
	if !slices.Equal(expected.Args, result.Args) {
		t.Errorf("Got wrong command args.  Expected %v, got %v", expected.Args, result.Args)
	}

	checkEnvVars := []string{
		"_condor_CREDD_HOST=mockCredd",
		"_condor_SEC_CREDENTIAL_GETTOKEN_OPTS=-a mockVaultServer",
	}
	for _, checkEnv := range checkEnvVars {
		if !slices.Contains(result.Env, checkEnv) {
			t.Errorf("Result cmd does not have right environment variables.  Missing %s", checkEnv)
		}
	}
}

func TestVaultStorerClientGetCmdArgs(t *testing.T) {
	baseV := &VaultStorerClient{
		credd:              "test.credd",
		vaultServer:        "test.vault.server",
		CommandEnvironment: &environment.CommandEnvironment{},
	}

	serviceName := "testService"

	type testCase struct {
		description  string
		v            *VaultStorerClient
		expectedArgs []string
	}

	testCases := []testCase{
		{
			"No verbose",
			baseV,
			[]string{serviceName},
		},
		{
			"Verbose",
			func(t *testing.T) *VaultStorerClient { return copyTestVaultStorerClient(baseV).WithVerbose() }(t),
			[]string{"-v", serviceName},
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				if cmdArgs := test.v.getCmdArgs(context.Background(), serviceName); !slices.Equal(cmdArgs, test.expectedArgs) {
					t.Errorf("cmdArgs slices are not equal. Expected %v, got %v", test.expectedArgs, cmdArgs)
				}
			},
		)
	}

}

func TestVaultStorerClientSetupCmdEnvironment(t *testing.T) {
	baseV := &VaultStorerClient{
		credd:              "test.credd",
		vaultServer:        "test.vault.server",
		CommandEnvironment: &environment.CommandEnvironment{},
	}

	type testCase struct {
		description     string
		v               *VaultStorerClient
		expectedEnvfunc func() *environment.CommandEnvironment
	}

	testCases := []testCase{
		{
			"Empty original env and old _condor_SEC_CREDENTIAL_GETTOKEN_OPTS",
			copyTestVaultStorerClient(baseV),
			func() *environment.CommandEnvironment {
				env := new(environment.CommandEnvironment)
				env.SetCondorCreddHost(baseV.credd)
				env.SetCondorSecCredentialGettokenOpts(fmt.Sprintf("-a %s", baseV.vaultServer))
				return env
			},
		},
		{
			"Empty original env, filled old _condor_SEC_CREDENTIAL_GETTOKEN_OPTS",
			func() *VaultStorerClient {
				v := copyTestVaultStorerClient(baseV)
				v.CommandEnvironment.SetCondorSecCredentialGettokenOpts("--foo bar")
				return v
			}(),
			func() *environment.CommandEnvironment {
				env := new(environment.CommandEnvironment)
				env.SetCondorCreddHost(baseV.credd)
				env.SetCondorSecCredentialGettokenOpts(fmt.Sprintf("--foo bar -a %s", baseV.vaultServer))
				return env
			},
		},
		{
			"Filled original env, empty old _condor_SEC_CREDENTIAL_GETTOKEN_OPTS",
			func() *VaultStorerClient {
				v := copyTestVaultStorerClient(baseV)
				v.CommandEnvironment.SetKrb5ccname("blahblah", environment.FILE)
				return v
			}(),
			func() *environment.CommandEnvironment {
				env := new(environment.CommandEnvironment)
				env.SetKrb5ccname("blahblah", environment.FILE)
				env.SetCondorCreddHost(baseV.credd)
				env.SetCondorSecCredentialGettokenOpts(fmt.Sprintf("-a %s", baseV.vaultServer))
				return env
			},
		},
		{
			"Filled original env, filled old _condor_SEC_CREDENTIAL_GETTOKEN_OPTS",
			func() *VaultStorerClient {
				v := copyTestVaultStorerClient(baseV)
				v.CommandEnvironment.SetKrb5ccname("blahblah", environment.FILE)
				v.CommandEnvironment.SetCondorSecCredentialGettokenOpts("--foo bar")
				return v
			}(),
			func() *environment.CommandEnvironment {
				env := new(environment.CommandEnvironment)
				env.SetKrb5ccname("blahblah", environment.FILE)
				env.SetCondorCreddHost(baseV.credd)
				env.SetCondorSecCredentialGettokenOpts(fmt.Sprintf("--foo bar -a %s", baseV.vaultServer))
				return env
			},
		},
	}

	for _, test := range testCases {
		t.Run(
			test.description,
			func(t *testing.T) {
				expectedEnv := test.expectedEnvfunc()
				if resultEnv := test.v.setupCmdEnvironment(); *resultEnv != *expectedEnv {
					t.Errorf("Did not get expected environmnent.  Expected %s, got %s", expectedEnv.String(), resultEnv.String())
				}
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

func TestNewVaultStorerClient(t *testing.T) {
	initialCredd := "credd1"
	vaultServer := "vaultServer.host"
	// TODO: Good test cases, but want to check entire VaultStorerClient struct
	type testCase struct {
		description               string
		initialCreddHost          string
		inputCredd                string
		expectedVaultStorerClient *VaultStorerClient
	}

	testCases := []testCase{
		{
			"Env credd matches input credd",
			initialCredd,
			initialCredd,
			&VaultStorerClient{
				credd:       initialCredd,
				vaultServer: vaultServer,
				CommandEnvironment: func() *environment.CommandEnvironment {
					env := &environment.CommandEnvironment{}
					env.SetCondorCreddHost(initialCredd)
					return env
				}(),
			},
		},
		{
			"Env credd differs from input credd",
			initialCredd,
			"credd2",
			&VaultStorerClient{
				credd:       "credd2",
				vaultServer: vaultServer,
				CommandEnvironment: func() *environment.CommandEnvironment {
					env := &environment.CommandEnvironment{}
					env.SetCondorCreddHost("credd2")
					return env
				}(),
			},
		},
		{
			"Env credd is empty, input credd is set",
			"",
			"credd3",
			&VaultStorerClient{
				credd:       "credd3",
				vaultServer: vaultServer,
				CommandEnvironment: func() *environment.CommandEnvironment {
					env := &environment.CommandEnvironment{}
					env.SetCondorCreddHost("credd3")
					return env
				}(),
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			origEnv := &environment.CommandEnvironment{}
			if test.initialCreddHost != "" {
				origEnv.SetCondorCreddHost(test.initialCreddHost)
			}
			client := NewVaultStorerClient(test.inputCredd, vaultServer, origEnv)
			if ((client).credd != (test.expectedVaultStorerClient).credd) ||
				((client).vaultServer != (test.expectedVaultStorerClient).vaultServer) ||
				((client).verbose != (test.expectedVaultStorerClient).verbose) ||
				(*((client).CommandEnvironment)) != *(test.expectedVaultStorerClient.CommandEnvironment) {
				t.Errorf("Expected VaultStorerClient %v, got %v", test.expectedVaultStorerClient, client)
			}
		})
	}
}

func copyTestVaultStorerClient(v *VaultStorerClient) *VaultStorerClient {
	return &VaultStorerClient{
		credd:              v.credd,
		vaultServer:        v.vaultServer,
		CommandEnvironment: v.CommandEnvironment.Copy(),
	}
}
