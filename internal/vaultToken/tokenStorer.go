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
	"regexp"

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

// TODO.  See if we can set this up like the htgettokenclient code.  Where we have a single entry point here, with an interactive bool that the caller can
// set to indicate whether the token storing action should be interactive or not.  Then we can get rid of a lot of this boilerplate and the caller just has to know
// to call a single function.

// // StoreAndValidateToken stores a vault token in the passed in Hashicorp vault server and the passed in credd.
// func StoreAndValidateToken[T *InteractiveTokenStorer | *NonInteractiveTokenStorer](ctx context.Context, t T, environ *environment.CommandEnvironment) error {
// 	switch val := any(t).(type) { // Workaround needed do type switch on constrained type
// 	case *InteractiveTokenStorer:
// 		return storeAndValidateToken(ctx, val, environ)
// 	case *NonInteractiveTokenStorer:
// 		return storeAndValidateToken(ctx, val, environ)
// 	}
// 	return errors.New("invalid token storer type")
// }

// func storeAndValidateToken(ctx context.Context, t tokenStorer, environ *environment.CommandEnvironment) error {
// 	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.StoreAndValidateToken")
// 	span.SetAttributes(
// 		attribute.String("service", t.GetServiceName()),
// 		attribute.String("credd", t.GetCredd()),
// 		attribute.String("vaultServer", t.GetVaultServer()),
// 	)
// 	defer span.End()

// 	funcLogger := log.WithFields(log.Fields{
// 		"serviceName": t.GetServiceName(),
// 		"vaultServer": t.GetVaultServer(),
// 		"credd":       t.GetCredd(),
// 	})
// 	if err := t.getTokensAndStoreInVault(ctx, environ); err != nil {
// 		if ctx.Err() == context.DeadlineExceeded {
// 			tracing.LogErrorWithTrace(span, funcLogger, "Context timeout")
// 			return err
// 		}
// 		tracing.LogErrorWithTrace(span, funcLogger, fmt.Sprintf("Could not obtain vault token: %s", err.Error()))
// 		return err
// 	}
// 	funcLogger.Debug("Stored vault and bearer tokens in vault and condor_credd/schedd")

// 	if err := t.validateToken(); err != nil {
// 		tracing.LogErrorWithTrace(span, funcLogger, "Could not validate vault token for TokenStorer")
// 		return err
// 	}

// 	span.SetStatus(codes.Ok, "Stored and validated vault token")
// 	funcLogger.Debug("Validated vault token")
// 	return nil
// }

// // tokenStorer contains the methods needed to store a vault token in the condor credd and a hashicorp vault.
// type tokenStorer interface {
// 	GetServiceName() string
// 	GetCredd() string
// 	GetVaultServer() string
// 	getTokensAndStoreInVault(context.Context, *environment.CommandEnvironment) error
// 	validateToken() error
// }

// // InteractiveTokenStorer is a type to use when it is anticipated that the token storing action will require user interaction
// type InteractiveTokenStorer struct {
// 	serviceName string
// 	credd       string
// 	vaultServer string
// }

// func NewInteractiveTokenStorer(serviceName, credd, vaultServer string) *InteractiveTokenStorer {
// 	return &InteractiveTokenStorer{
// 		serviceName: serviceName,
// 		credd:       credd,
// 		vaultServer: vaultServer,
// 	}
// }

// func (t *InteractiveTokenStorer) GetServiceName() string { return t.serviceName }
// func (t *InteractiveTokenStorer) GetCredd() string       { return t.credd }
// func (t *InteractiveTokenStorer) GetVaultServer() string { return t.vaultServer }
// func (t *InteractiveTokenStorer) validateToken() error {
// 	return validateServiceVaultToken(t.serviceName)
// }

// // getTokensandStoreinVault stores a refresh token in a configured Hashicorp vault and obtains vault and bearer tokens for the user.
// // It allows for the token-storing command to prompt the user for action
// func (t *InteractiveTokenStorer) getTokensAndStoreInVault(ctx context.Context, environ *environment.CommandEnvironment) error {
// 	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.InteractiveTokenStorer.getTokensAndStoreInVault")
// 	span.SetAttributes(
// 		attribute.String("service", t.serviceName),
// 		attribute.String("credd", t.credd),
// 		attribute.String("vaultServer", t.vaultServer),
// 	)
// 	defer span.End()

// 	funcLogger := log.WithFields(log.Fields{
// 		"service":     t.serviceName,
// 		"vaultServer": t.vaultServer,
// 		"credd":       t.credd,
// 	})
// 	getTokensAndStoreInVaultCmd := setupCmdWithEnvironmentForTokenStorer(ctx, t, environ)

// 	// We need to capture stdout and stderr on the terminal so the user can authenticate
// 	getTokensAndStoreInVaultCmd.Stdout = os.Stdout
// 	getTokensAndStoreInVaultCmd.Stderr = os.Stderr

// 	if err := getTokensAndStoreInVaultCmd.Start(); err != nil {
// 		if ctx.Err() == context.DeadlineExceeded {
// 			tracing.LogErrorWithTrace(span, funcLogger, "Context timeout")
// 			return ctx.Err()
// 		}
// 		tracing.LogErrorWithTrace(span, funcLogger, fmt.Sprintf("Error starting condor_vault_storer command to store and obtain tokens; %s", err.Error()))
// 		return err
// 	}
// 	if err := getTokensAndStoreInVaultCmd.Wait(); err != nil {
// 		if ctx.Err() == context.DeadlineExceeded {
// 			tracing.LogErrorWithTrace(span, funcLogger, "Context timeout")
// 			return ctx.Err()
// 		}
// 		tracing.LogErrorWithTrace(span, funcLogger, fmt.Sprintf("Error running condor_vault_storer to store and obtain tokens; %s", err))
// 		return err
// 	}
// 	span.SetStatus(codes.Ok, "Stored and obtained vault token")
// 	return nil
// }

// // NonInteractiveTokenStorer is a type to use when it is anticipated that the token storing action will not require user interaction
// type NonInteractiveTokenStorer struct {
// 	serviceName string
// 	credd       string
// 	vaultServer string
// }

// func NewNonInteractiveTokenStorer(serviceName, credd, vaultServer string) *NonInteractiveTokenStorer {
// 	return &NonInteractiveTokenStorer{
// 		serviceName: serviceName,
// 		credd:       credd,
// 		vaultServer: vaultServer,
// 	}
// }

// func (t *NonInteractiveTokenStorer) GetServiceName() string { return t.serviceName }
// func (t *NonInteractiveTokenStorer) GetCredd() string       { return t.credd }
// func (t *NonInteractiveTokenStorer) GetVaultServer() string { return t.vaultServer }
// func (t *NonInteractiveTokenStorer) validateToken() error {
// 	return validateServiceVaultToken(t.serviceName)
// }

// // getTokensandStoreinVault stores a refresh token in a configured Hashicorp vault and obtains vault and bearer tokens for the user.
// // It doew not allow for the token-storing command to prompt the user for action
// func (t *NonInteractiveTokenStorer) getTokensAndStoreInVault(ctx context.Context, environ *environment.CommandEnvironment) error {
// 	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.NonInteractiveTokenStorer.getTokensAndStoreInVault")
// 	span.SetAttributes(
// 		attribute.String("service", t.serviceName),
// 		attribute.String("credd", t.credd),
// 		attribute.String("vaultServer", t.vaultServer),
// 	)
// 	defer span.End()

// 	funcLogger := log.WithFields(log.Fields{
// 		"service":     t.serviceName,
// 		"vaultServer": t.vaultServer,
// 		"credd":       t.credd,
// 	})
// 	getTokensAndStoreInVaultCmd := setupCmdWithEnvironmentForTokenStorer(ctx, t, environ)

// 	// For non-interactive use, it is an error condition if the user is prompted to authenticate, so we want to just run the command
// 	// and capture the output
// 	if stdoutStderr, err := getTokensAndStoreInVaultCmd.CombinedOutput(); err != nil {
// 		if ctx.Err() == context.DeadlineExceeded {
// 			tracing.LogErrorWithTrace(span, funcLogger, "Context timeout")
// 			return ctx.Err()
// 		}
// 		authErr := checkStdoutStderrForAuthNeededError(stdoutStderr)
// 		funcLogger.Errorf("Error running condor_vault_storer to store and obtain tokens; %s", err)
// 		funcLogger.Errorf("%s", stdoutStderr)
// 		if authErr != nil {
// 			span.SetStatus(codes.Error, "Authentication needed")
// 			return authErr
// 		}
// 		span.SetStatus(codes.Error, "Error running condor_vault_storer to store and obtain tokens")
// 		return err
// 	} else {
// 		if len(stdoutStderr) > 0 {
// 			funcLogger.Debugf("%s", stdoutStderr)
// 		}
// 	}
// 	span.SetStatus(codes.Ok, "Stored and obtained vault token")
// 	return nil
// }

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

func checkStdoutStderrForAuthNeededError(stdoutStderr []byte) error {
	authNeededRegexp := regexp.MustCompile(`Authentication needed for.*`)
	if !authNeededRegexp.Match(stdoutStderr) {
		return nil
	}

	errToReturn := &ErrAuthNeeded{}
	htgettokenTimeoutRegexp := regexp.MustCompile(`htgettoken: Polling for response took longer than.*`)
	if htgettokenTimeoutRegexp.Match(stdoutStderr) {
		errToReturn.underlyingError = errHtgettokenTimeout
	}
	htgettokenPermissionDeniedRegexp := regexp.MustCompile(`htgettoken:.*403.*permission denied`)
	if htgettokenPermissionDeniedRegexp.Match(stdoutStderr) {
		errToReturn.underlyingError = errHtgettokenPermissionDenied
	}

	return errToReturn
}

type ErrAuthNeeded struct {
	underlyingError error
}

func (e *ErrAuthNeeded) Error() string {
	msg := "authentication needed for service to generate vault token"
	if e.underlyingError != nil {
		msg = fmt.Sprintf("%s: %s", msg, e.underlyingError.Error())
	}
	return msg
}

func (e *ErrAuthNeeded) Unwrap() error { return e.underlyingError }

var (
	errHtgettokenTimeout          = errors.New("htgettoken timeout to generate vault token")
	errHtgettokenPermissionDenied = errors.New("permission denied to generate vault token")
)
