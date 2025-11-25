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

// Package vaultToken provides functions for obtaining and validating Hashicorp vault tokens using the configured HTCondor installation
package vaultToken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/lestrrat-go/jwx/jwt"
	scitokens "github.com/scitokens/scitokens-go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/fermitools/managed-tokens/internal/environment"
)

// Borrowed from hashicorp's vault API, since we ONLY need this func
// Source: https://github.com/hashicorp/vault/blob/main/vault/version_store.go
// and https://github.com/hashicorp/vault/blob/main/sdk/helper/consts/token_consts.go

const (
	ServiceTokenPrefix       = "hvs."
	LegacyServiceTokenPrefix = "s."
)

// IsServiceToken validates that a token string follows the Hashicorp service token convention
func IsServiceToken(token string) bool {
	return strings.HasPrefix(token, ServiceTokenPrefix) ||
		strings.HasPrefix(token, LegacyServiceTokenPrefix)
}

// InvalidVaultTokenError is an error that indicates that the token contained in filename is not a valid Hashicorp Service Token
// (what is called a vault token in the managed-tokens/OSG/WLCG world)
type InvalidVaultTokenError struct {
	filename string
	msg      string
}

func (i *InvalidVaultTokenError) Error() string {
	return fmt.Sprintf(
		"%s is an invalid vault/service token. %s",
		i.filename,
		i.msg,
	)
}

// GetAllVaultTokenLocations returns the locations of the vault tokens that both HTCondor and other OSG grid tools will use.
// The first element of the returned slice is the standard location for most grid tools, and the second is the standard for
// HTCondor
func GetAllVaultTokenLocations(serviceName string) ([]string, error) {
	funcLogger := log.WithField("service", serviceName)
	vaultTokenLocations := make([]string, 0, 2)

	defaultLocation, err := getDefaultVaultTokenLocation()
	if err != nil {
		funcLogger.Error("Could not get default vault location")
		return nil, err
	}
	if _, err := os.Stat(defaultLocation); err == nil { // Check to see if the file exists and we can read it
		vaultTokenLocations = append(vaultTokenLocations, defaultLocation)
	}

	condorLocation, err := getCondorVaultTokenLocation(serviceName)
	if err != nil {
		funcLogger.Error("Could not get condor vault location")
		return nil, err
	}
	if _, err := os.Stat(condorLocation); err == nil { // Check to see if the file exists and we can read it
		vaultTokenLocations = append(vaultTokenLocations, condorLocation)
	}

	return vaultTokenLocations, nil
}

// RemoveServiceVaultTokens removes the vault token files at the standard OSG Grid Tools and HTCondor locations
func RemoveServiceVaultTokens(serviceName string) error {
	vaultTokenLocations, err := GetAllVaultTokenLocations(serviceName)
	if err != nil {
		log.WithField("service", serviceName).Error("Could not get vault token locations for deletion")
	}
	for _, vaultToken := range vaultTokenLocations {
		tokenLogger := log.WithFields(log.Fields{
			"service":  serviceName,
			"filename": vaultToken,
		})
		if err := os.Remove(vaultToken); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				tokenLogger.Info("Vault token not removed because the file does not exist")
			} else {
				tokenLogger.Error("Could not remove vault token")
				return err
			}
		} else {
			tokenLogger.Debug("Removed vault token")
		}
	}
	return nil
}

// getCondorVaultTokenLocation returns the location of vault token that HTCondor uses based on the current user's UID
func getCondorVaultTokenLocation(serviceName string) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		log.WithField("service", serviceName).Error(err)
		return "", err
	}
	currentUID := currentUser.Uid
	return fmt.Sprintf("/tmp/vt_u%s-%s", currentUID, serviceName), nil
}

// getDefaultVaultTokenLocation returns the location of vault token that most OSG grid tools use based on the current user's UID
func getDefaultVaultTokenLocation() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		log.Error(err)
		return "", err
	}
	currentUID := currentUser.Uid
	return fmt.Sprintf("/tmp/vt_u%s", currentUID), nil
}

// getDefaultBearerTokenFileLocation returns the default location for the bearer token file, following the logic of the WLCG Bearer Token Discovery specification:
// 1. If the BEARER_TOKEN_FILE environment variable is set, use that
// 2. If the XDG_RUNTIME_DIR environment variable is set, use $XDG_RUNTIME_DIR/bt_u<uid>
// 3. Otherwise, use /tmp/bt_u<uid>
//
// See https://zenodo.org/records/3937438 for more details
func getDefaultBearerTokenFileLocation() (string, error) {
	if f, ok := os.LookupEnv("BEARER_TOKEN_FILE"); ok {
		return f, nil
	}
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	currentUID := currentUser.Uid

	if d, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		return filepath.Join(d, fmt.Sprintf("/bt_u%s", currentUID)), nil
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("/bt_u%s", currentUID)), nil
}

func validateServiceVaultToken(serviceName string) error {
	funcLogger := log.WithField("service", serviceName)
	vaultTokenFilename, err := getCondorVaultTokenLocation(serviceName)
	if err != nil {
		funcLogger.Error("Could not get default vault token location")
		return err
	}

	if err := validateVaultToken(vaultTokenFilename); err != nil {
		funcLogger.Error("Could not validate vault token")
		return err
	}
	return nil
}

// validateVaultToken verifies that a vault token (service token as Hashicorp calls them) indicated by the filename is valid
func validateVaultToken(vaultTokenFilename string) error {
	vaultTokenBytes, err := os.ReadFile(vaultTokenFilename)
	if err != nil {
		log.WithField("filename", vaultTokenFilename).Error("Could not read tokenfile for verification.")
		return err
	}

	vaultTokenString := string(vaultTokenBytes[:])

	if !IsServiceToken(vaultTokenString) {
		errString := "vault token failed validation"
		log.WithField("filename", vaultTokenFilename).Error(errString)
		return &InvalidVaultTokenError{
			vaultTokenFilename,
			errString,
		}
	}
	return nil
}

// // TODO STILL UNDER DEVELOPMENT.  Export when ready, and add tracing
// func GetToken(ctx context.Context, userPrincipal, serviceName, vaultServer string, environ environment.CommandEnvironment) error {
// 	htgettokenArgs := []string{
// 		"-d",
// 		"-a",
// 		vaultServer,
// 		"-i",
// 		serviceName,
// 	}

// 	htgettokenCmd := environment.EnvironmentWrappedCommand(ctx, &environ, vaultExecutables["htgettoken"], htgettokenArgs...)
// 	// TODO Get rid of all this when it works
// 	htgettokenCmd.Stdout = os.Stdout
// 	htgettokenCmd.Stderr = os.Stderr
// 	log.Debug(htgettokenCmd.Args)

// 	log.WithField("service", serviceName).Info("Running htgettoken to get vault and bearer tokens")
// 	if stdoutStderr, err := htgettokenCmd.CombinedOutput(); err != nil {
// 		if ctx.Err() == context.DeadlineExceeded {
// 			log.WithField("service", serviceName).Error("Context timeout")
// 			return ctx.Err()
// 		}
// 		log.WithField("service", serviceName).Errorf("Could not get vault token:\n%s", string(stdoutStderr[:]))
// 		return err
// 	}

// 	log.WithField("service", serviceName).Debug("Successfully got vault token")
// 	return nil
// }

// Notes:
// 1. getDefaultVaultTokenLocation is already defined above and gets the vault token location we need
// 2. Add worker type "GetTokenWorker" to just get token.  It will have to instantiate the htgettokenClient, move in the staged vault token,
//   call GetToken, and then validate the token.
//  The worker can just throw out the BEARER TOKEN, so we can write the token to a tempfile and delete it after validation
// 3.  Maybe all the workers could be combined into an interface, with a type switch to determine which worker to use?
// 4. CondorVaultTokenLocation is defined here and in the worker package.  We only need it here?, and export it perhaps

// From caller POV:
// c := NewHtgettokenClient(stuff)
// err := GetToken(ctx, stuff, c)

// OR:
//// func NewHtgettokenClient(stuff any) HtgettokenClient { }
// func (h *HtgettokenClient) GetToken(ctx context.Context, issuer, role string) error {
// 		Setup
//    Logic to handle interactive or not
//
// }

// Heavily borrowed from https://github.com/fnal-fife/jobsub-pnfs-dropbox-cleanup/blob/5168a5d0fc30284fa22350bcee42e55687f532eb/htgettokenClient.go

// HtgettokenClient is a client for interacting with the htgettoken command-line tool.
type HtgettokenClient struct {
	// vaultServer is the Vault server URL to use for authentication.
	vaultServer string
	// vaultTokenFile is the path to the file that contains the vault token used to authorize vault operations
	vaultTokenFile string
	// outFile is the path where a bearer token will be written to. If the file does not exist, it will be created.
	outFile            string
	options            []string
	verbose            bool // Whether to enable verbose mode for htgettoken
	CommandEnvironment *environment.CommandEnvironment
}

// NewHtgettokenClient creates a new htgettokenClient instance.
// outFile and options are optional - if not provided, they will be set to default values.
// The HTGETTOKENOPTS environment variable should be set in the CommandEnvironment if needed, like this:
//
//	c := environment.CommandEnvironment{}
//	c.SetHtgettokenOpts("value")
func NewHtgettokenClient(vaultServer, vaultTokenFile, outFile string, env *environment.CommandEnvironment, options ...string) (*HtgettokenClient, error) {
	if vaultServer == "" {
		return nil, errors.New("vault server cannot be empty")
	}

	var err error

	var useVaultTokenFile string = vaultTokenFile
	if vaultTokenFile == "" {
		useVaultTokenFile, err = getDefaultVaultTokenLocation()
		if err != nil {
			return nil, fmt.Errorf("vault token file not given and there was error getting default vault token location: %w", err)
		}
	}

	var useOutFile string = outFile
	if outFile == "" {
		useOutFile, err = getDefaultBearerTokenFileLocation()
		if err != nil {
			return nil, fmt.Errorf("output file not given and there was error getting default output file location: %w", err)
		}
	}

	return &HtgettokenClient{
		vaultServer:        vaultServer,
		vaultTokenFile:     useVaultTokenFile,
		outFile:            useOutFile,
		options:            options,
		CommandEnvironment: env,
	}, nil

}

func (h *HtgettokenClient) WithVerbose() *HtgettokenClient {
	// Add the --verbose flag to the options
	h.verbose = true
	return h
}

func (h *HtgettokenClient) GetToken(ctx context.Context, issuer, role string, interactive bool) error {
	funcLogger := log.WithField("caller", "HtgettokenClient.GetToken")
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

	cmdArgs := h.prepareCmdArgs(issuer, role)

	cmd := environment.EnvironmentWrappedCommand(ctx, h.CommandEnvironment, vaultExecutables["htgettoken"], cmdArgs...)
	funcLogger.Debug("Running htgettoken command", "command", vaultExecutables["htgettoken"], "args", cmdArgs, "env", cmd.Env)

	err := runner.executeCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("error running htgettoken to obtain bearer token: %w", err)
	}

	// Check token - if there's an error here, we want the warning, but not to stop execution
	if err := checkToken(h.outFile, issuer, role); err != nil {
		funcLogger.Warn("error checking token", "tokenfile", h.outFile, "error", err)
	}

	return nil
}

func (h *HtgettokenClient) prepareCmdArgs(issuer, role string) []string {
	cmdArgs := []string{
		"-a",
		h.vaultServer,
		"-i",
		issuer,
		"--vaulttokenfile",
		h.vaultTokenFile,
		"-o",
		h.outFile,
	}
	cmdArgs = append(cmdArgs, h.options...)

	if h.verbose {
		cmdArgs = append(cmdArgs, "--verbose")
	}

	if role != "" {
		cmdArgs = append(cmdArgs, "--role", role)
	}

	return cmdArgs
}

func checkToken(tokenFile, issuer, role string) error {
	// Read token in tokenFile in, validate it as a SciToken, and return it
	funcLogger := log.WithFields(log.Fields{
		"tokenFile": tokenFile,
		"issuer":    issuer,
		"role":      role,
	})
	errValidateMsg := "error validating token"

	tok, err := os.ReadFile(tokenFile)
	if err != nil {
		funcLogger.Errorf("error reading token file %s:", err)
		return fmt.Errorf("%s: %w", errValidateMsg, err)
	}

	// Parse the token to verify that it's a valid JWT
	jt, err := jwt.Parse(tok)
	if err != nil {
		funcLogger.Errorf("error parsing token: %s", err)
		return fmt.Errorf("%s: %w", errValidateMsg, err)
	}

	// Convert our token to a SciToken
	st, err := scitokens.NewSciToken(jt)
	if err != nil {
		funcLogger.Errorf("error creating SciToken from token file: %s", err)
		return fmt.Errorf("%s: %w", errValidateMsg, err)
	}

	enf, err := scitokens.NewEnforcer(st.Issuer())
	if err != nil {
		funcLogger.Error("error creating SciToken from token", "tokenfile", tokenFile, "error", err)
		return fmt.Errorf("%s: %w", errValidateMsg, err)
	}

	// Validate the token
	validators := []scitokens.Validator{scitokens.WithGroup(issuer)}
	if role != "" {
		validators = append(validators, scitokens.WithGroup(fmt.Sprintf("%s/%s", issuer, role)))
	}

	if err = enf.Validate(st, validators...); err != nil {
		funcLogger.Error("error validating SciToken file", "tokenfile", tokenFile, "error", err)
		return fmt.Errorf("%s: %w", errValidateMsg, err)
	}

	return nil
}

type commandExecutor interface {
	executeCommand(ctx context.Context, c *exec.Cmd) error
}

type interactiveExecutor struct{}
type nonInteractiveExecutor struct{}

func (i *interactiveExecutor) executeCommand(ctx context.Context, c *exec.Cmd) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.interactiveExecutor.executeCommand")
	span.SetAttributes(
		attribute.String("command", c.String()),
	)
	defer span.End()

	// We want to run the command interactively, so set up Stdout and Stderr
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Start(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			span.SetStatus(codes.Error, "Context timeout")
			return ctx.Err()
		}
		span.SetStatus(codes.Error, "Error starting command")
		return err
	}

	if err := c.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			span.SetStatus(codes.Error, "Context timeout")
			return ctx.Err()
		}
		span.SetStatus(codes.Error, fmt.Sprintf("Error waiting for command; %s", err.Error()))
		return err
	}

	span.SetStatus(codes.Ok, "Command executed successfully")
	return nil
}

// TODO test
// Mock authNeeded, or regular error, or it works, or timeout
func (n *nonInteractiveExecutor) executeCommand(ctx context.Context, c *exec.Cmd) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.nonInteractiveExecutor.executeCommand")
	span.SetAttributes(
		attribute.String("command", c.String()),
	)
	defer span.End()
	funcLogger := log.WithField("caller", "vaultToken.nonInteractiveExecutor.executeCommand")

	if stdoutStderr, err := c.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			span.SetStatus(codes.Error, "Context timeout")
			return ctx.Err()
		}
		funcLogger.Errorf("%s", stdoutStderr)
		authErr := checkStdoutStderrForAuthNeededError(stdoutStderr)
		if authErr != nil {
			span.SetStatus(codes.Error, "Authentication needed")
			return authErr
		}
		span.SetStatus(codes.Error, "Command execution failed")
		return err
	} else if len(stdoutStderr) > 0 {
		funcLogger.Debugf("%s", string(stdoutStderr))
	}
	span.SetStatus(codes.Ok, "Command executed successfully")
	return nil
}
