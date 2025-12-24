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

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/fermitools/managed-tokens/internal/environment"
	"github.com/fermitools/managed-tokens/internal/tracing"
	"github.com/fermitools/managed-tokens/internal/utils"
)

var vaultExecutables = map[string]string{
	"condor_vault_storer": "",
	"condor_store_cred":   "",
	"condor_status":       "",
	"htgettoken":          "",
}

func init() {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:/usr/bin:/usr/sbin", oldPath))
	if err := utils.CheckForExecutables(vaultExecutables); err != nil {
		log.WithField("PATH", os.Getenv("PATH")).Fatal("Could not find path to condor executables")
	}
}

type VaultStorerClient struct {
	credd              string
	vaultServer        string
	verbose            bool // Whether to enable verbose mode for vault storer command
	CommandEnvironment *environment.CommandEnvironment
}

func NewVaultStorerClient(credd, vaultServer string, environ *environment.CommandEnvironment) *VaultStorerClient {
	// If environ has a different _condor_CREDD_HOST than credd, make a copy of environ and set _condor_CREDD_HOST to credd
	useEnv := environ
	if _credd := environ.GetValue(environment.CondorCreddHost); _credd != credd {
		useEnv = environ.Copy()
		useEnv.SetCondorCreddHost(credd)
	}
	return &VaultStorerClient{
		credd:              credd,
		vaultServer:        vaultServer,
		CommandEnvironment: useEnv,
	}
}

func (v *VaultStorerClient) WithVerbose() *VaultStorerClient {
	v.verbose = true
	return v
}

func (v *VaultStorerClient) GetCredd() string       { return v.credd }
func (v *VaultStorerClient) GetVaultServer() string { return v.vaultServer }

// GetAndStoreToken gets and stores a vault token for the given serviceName in the configured vault server and credd.
// If interactive is true, the command may prompt the user for action if needed.
func (v *VaultStorerClient) GetAndStoreToken(ctx context.Context, serviceName string, interactive bool) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.VaultStorerClient.GetAndStoreToken")
	span.SetAttributes(
		attribute.String("service", serviceName),
		attribute.String("credd", v.credd),
		attribute.String("vaultServer", v.vaultServer),
	)
	defer span.End()

	funcLogger := log.WithFields(log.Fields{
		"serviceName": serviceName,
		"vaultServer": v.vaultServer,
		"credd":       v.credd,
	})

	// Check the context before proceeding
	if err := ctx.Err(); err != nil {
		msg := "context deadline exceeded before getting token"
		if errors.Is(err, context.Canceled) {
			msg = "context canceled before getting token"
			funcLogger.Error(msg, " error:", err)
			return fmt.Errorf("%s: %w", msg, err)
		}
		return fmt.Errorf("%s: %w", msg, err)
	}

	var runner commandExecutor = &nonInteractiveExecutor{} // By default, use non-interactive executor
	if interactive {
		runner = &interactiveExecutor{}
	}

	cmd := v.setupCmdWithEnvironment(ctx, serviceName)
	if err := runner.executeCommand(ctx, cmd); err != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "Could not get and store vault token")
		return fmt.Errorf("error getting and storing vault token on credd: %w", err)
	}

	return nil
}

func (v *VaultStorerClient) setupCmdWithEnvironment(ctx context.Context, serviceName string) *exec.Cmd {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.setupCmdWithEnvironmentForTokenStorer")
	span.SetAttributes(
		attribute.String("service", serviceName),
		attribute.String("credd", v.credd),
		attribute.String("vaultServer", v.vaultServer),
	)
	defer span.End()

	funcLogger := log.WithFields(log.Fields{
		"service":     serviceName,
		"vaultServer": v.vaultServer,
		"credd":       v.credd,
	})

	cmdArgs := v.getCmdArgs(ctx, serviceName)
	newEnv := v.setupCmdEnvironment()
	getTokensAndStoreInVaultCmd := environment.EnvironmentWrappedCommand(ctx, newEnv, vaultExecutables["condor_vault_storer"], cmdArgs...)

	funcLogger.Info("Storing and obtaining vault token")
	funcLogger.WithFields(log.Fields{
		"command":     getTokensAndStoreInVaultCmd.String(),
		"environment": newEnv.String(),
	}).Debug("Command to store vault token")

	return getTokensAndStoreInVaultCmd
}

func (v *VaultStorerClient) getCmdArgs(ctx context.Context, serviceName string) []string {
	cmdArgs := make([]string, 0, 2)
	if v.verbose {
		cmdArgs = append(cmdArgs, "-v")
	}
	cmdArgs = append(cmdArgs, serviceName)
	return cmdArgs
}

// setupEnvironment sets _condor_CREDD_HOST and _condor_SEC_CREDENTIAL_GETTOKEN_OPTS in a new environment for condor_vault_storer
func (v *VaultStorerClient) setupCmdEnvironment() *environment.CommandEnvironment {
	newEnv := v.CommandEnvironment.Copy()
	newEnv.SetCondorCreddHost(v.credd)
	oldCondorSecCredentialGettokenOpts := v.CommandEnvironment.GetValue(environment.CondorSecCredentialGettokenOpts)
	var maybeSpace string
	if oldCondorSecCredentialGettokenOpts != "" {
		maybeSpace = " "
	}
	newEnv.SetCondorSecCredentialGettokenOpts(oldCondorSecCredentialGettokenOpts + maybeSpace + fmt.Sprintf("-a %s", v.vaultServer))
	return newEnv
}

var (
	errHtgettokenTimeout          = errors.New("htgettoken timeout to generate vault token")
	errHtgettokenPermissionDenied = errors.New("permission denied to generate vault token")
)
