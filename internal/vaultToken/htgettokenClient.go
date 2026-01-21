// COPYRIGHT 2025 FERMI NATIONAL ACCELERATOR LABORATORY
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
	"os/user"
	"path/filepath"

	"github.com/lestrrat-go/jwx/jwt"
	scitokens "github.com/scitokens/scitokens-go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/fermitools/managed-tokens/internal/environment"
)

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

// WithVerbose enables verbose mode for the htgettoken command.
func (h *HtgettokenClient) WithVerbose() *HtgettokenClient {
	// Add the --verbose flag to the options
	h.verbose = true
	return h
}

// GetToken retrieves a bearer token from the Vault server using the htgettoken command. The issuer, like in the htgettoken command, refers not to
// the token's "iss" claim, but to the Vault/OpenBao-configured "issuer" key of the token issuer
func (h *HtgettokenClient) GetToken(ctx context.Context, issuer, role string, interactive bool) ([]byte, error) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "vaultToken.HtgettokenClient.GetToken")
	span.SetAttributes(
		attribute.String("issuer", issuer),
		attribute.String("role", role),
	)
	defer span.End()
	funcLogger := log.WithField("caller", "HtgettokenClient.GetToken")

	// Check the context before proceeding
	if err := ctx.Err(); err != nil {
		msg := "context deadline exceeded before getting token"
		if errors.Is(err, context.Canceled) {
			msg = "context canceled before getting token"
			funcLogger.Error(msg, " error:", err)
			return nil, fmt.Errorf("%s: %w", msg, err)
		}
		return nil, fmt.Errorf("%s: %w", msg, err)
	}

	var runner commandExecutor = &nonInteractiveExecutor{} // By default, use non-interactive executor
	if interactive {
		runner = &interactiveExecutor{}
	}

	cmdArgs := h.prepareCmdArgs(issuer, role)

	cmd := environment.EnvironmentWrappedCommand(ctx, h.CommandEnvironment, vaultExecutables["htgettoken"], cmdArgs...)
	funcLogger.WithFields(log.Fields{
		"command":            vaultExecutables["htgettoken"],
		"args":               cmdArgs,
		"commandEnvironment": h.CommandEnvironment,
	}).Debug("Running htgettoken command")

	err := runner.executeCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("error running htgettoken to obtain bearer token: %w", err)
	}

	// Check token - if there's an error here, we want the warning, but not to stop execution
	if err := checkToken(h.outFile, issuer, role); err != nil {
		funcLogger.Warn("error checking token", "tokenfile", h.outFile, "error", err)
	}

	tokenBytes, err := os.ReadFile(h.outFile)
	if err != nil {
		return nil, fmt.Errorf("error reading token outfile: %w", err)
	}

	return tokenBytes, nil
}

// prepareCmdArgs prepares the command-line arguments for the htgettoken command.
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

// checkToken reads the token from tokenFile, validates it as a SciToken with the given issuer and role, and returns an error if validation fails.
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
