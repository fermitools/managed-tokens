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
	"os/exec"
	"regexp"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// commandExecutor defines an interface for executing commands.
type commandExecutor interface {
	executeCommand(ctx context.Context, c *exec.Cmd) error
}

// interactiveExecutor executes commands interactively, allowing for user input if needed
type interactiveExecutor struct{}

// executeCommand runs the provided command, allowing for user input if needed. If the command
// exits with a non-zero exit code or there is otherwise an error running the command, a
// non-nil error is returned
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

// nonInteractiveExecutor executes commands non-interactively, and assumes that no user input will be needed
type nonInteractiveExecutor struct{}

// executeCommand runs the provided command, assuming no user input will be needed. If the command exits with a non-zero exit
// code or there is otherwise an error running the command, or the command waits for user input, a non-nil error is returned
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

// checkStdoutStderrForAuthNeededError inspects the provided stdout and stderr output for authentication-related errors.
// If an authentication error is detected, it returns an ErrAuthNeeded, optionally wrapping a more specific underlying error
// such as timeout or permission denied. If no authentication error is found, it returns nil.
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

// ErrAuthNeeded represents an error indicating that authentication is required.
// It wraps an underlying error that provides more context about the authentication failure.
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
