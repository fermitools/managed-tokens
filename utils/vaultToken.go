package utils

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/shreyb/managed-tokens/service"
)

var vaultExecutables = map[string]string{
	"condor_vault_storer": "",
	"condor_store_cred":   "",
	"htgettoken":          "",
}

func init() {
	os.Setenv("PATH", "/usr/bin:/usr/sbin")
	if err := CheckForExecutables(vaultExecutables); err != nil {
		log.Fatal("Could not find path to condor executables")
	}
}

func StoreAndGetTokens(ctx context.Context, sc *service.Config, interactive bool) error {
	// kswitch
	// TODO:  Change this to passed in context later
	if err := SwitchKerberosCache(ctx, sc); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Error("Context timeout")
			return ctx.Err()
		}
		log.WithFields(log.Fields{
			"experiment": sc.Service.Experiment(),
			"role":       sc.Service.Role(),
		}).Error("Could not switch kerberos caches")
		return err
	}

	// Get token and store it in vault
	if err := getTokensandStoreinVault(ctx, sc, interactive); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Error("Context timeout")
			return ctx.Err()
		}
		log.WithFields(log.Fields{
			"experiment": sc.Service.Experiment(),
			"role":       sc.Service.Role(),
		}).Error("Could not obtain vault token")
		return err
	}

	// Validate vault token
	currentUser, err := user.Current()
	if err != nil {
		log.WithFields(log.Fields{
			"experiment": sc.Service.Experiment(),
			"role":       sc.Service.Role(),
		}).Error(err)
		return err
	}
	currentUID := currentUser.Uid
	vaultTokenFilename := fmt.Sprintf("/tmp/vt_u%s-%s", currentUID, sc.Service.Name())

	if err := validateVaultToken(vaultTokenFilename); err != nil {
		log.WithFields(log.Fields{
			"experiment": sc.Service.Experiment(),
			"role":       sc.Service.Role(),
		}).Error("Could not validate vault token")
		return err
	}

	// TODO Make this a debug
	log.WithFields(log.Fields{
		"experiment": sc.Service.Experiment(),
		"role":       sc.Service.Role(),
	}).Info("Validated vault token")
	return nil
}

func GetToken(ctx context.Context, sc *service.Config, vaultServer string) error {
	// TODO:  CHange this to passed in context later
	if err := SwitchKerberosCache(ctx, sc); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Error("Context timeout")
			return ctx.Err()
		}
		log.WithFields(log.Fields{
			"experiment": sc.Service.Experiment(),
			"role":       sc.Service.Role(),
		}).Error("Could not switch kerberos caches")
		return err
	}

	htgettokenArgs := []string{
		"-d",
		"-a",
		vaultServer,
		"-i",
		sc.Service.Experiment(),
	}

	if sc.Service.Role() != service.DefaultRole {
		htgettokenArgs = append(htgettokenArgs, []string{"-r", sc.Service.Role()}...)
	}

	htgettokenCmd := exec.CommandContext(ctx, vaultExecutables["htgettoken"], htgettokenArgs...)
	htgettokenCmd = EnvironmentWrappedCommand(htgettokenCmd, &sc.CommandEnvironment)
	// Get rid of all this when it works
	htgettokenCmd.Stdout = os.Stdout
	htgettokenCmd.Stderr = os.Stderr
	log.Debug(htgettokenCmd.Args)

	log.WithField("service", sc.Service.Name()).Info("Running htgettoken to get vault and bearer tokens")
	if stdoutStderr, err := htgettokenCmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Error("Context timeout")
			return ctx.Err()
		}
		log.WithField("service", sc.Service.Name()).Error("Could not get vault token")
		log.WithField("service", sc.Service.Name()).Error(string(stdoutStderr[:]))
		return err
	}

	log.WithField("service", sc.Service.Name()).Info("Successfully got vault token")
	return nil
}

func getTokensandStoreinVault(ctx context.Context, sc *service.Config, interactive bool) error {
	// Store token in vault and get new vault token
	//TODO if verbose, add the -v flag here
	getTokensAndStoreInVaultCmd := exec.CommandContext(ctx, vaultExecutables["condor_vault_storer"], sc.Service.Name())
	getTokensAndStoreInVaultCmd = EnvironmentWrappedCommand(getTokensAndStoreInVaultCmd, &sc.CommandEnvironment)

	log.WithFields(log.Fields{
		"experiment": sc.Service.Experiment(),
		"role":       sc.Service.Role(),
	}).Info("Storing and obtaining vault token")

	if interactive {
		// We need to capture stdout and stderr on the terminal so the user can authenticate
		getTokensAndStoreInVaultCmd.Stdout = os.Stdout
		getTokensAndStoreInVaultCmd.Stderr = os.Stderr

		if err := getTokensAndStoreInVaultCmd.Start(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.WithFields(log.Fields{
					"experiment": sc.Service.Experiment(),
					"role":       sc.Service.Role(),
				}).Error("Context timeout")
				return ctx.Err()
			}
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Errorf("Error starting condor_vault_storer command to store and obtain tokens; %s", err.Error())
		}
		if err := getTokensAndStoreInVaultCmd.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.WithFields(log.Fields{
					"experiment": sc.Service.Experiment(),
					"role":       sc.Service.Role(),
				}).Error("Context timeout")
				return ctx.Err()
			}
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Errorf("Error running condor_vault_storer to store and obtain tokens; %s", err.Error())
			return err
		}
	} else {
		if stdoutStderr, err := getTokensAndStoreInVaultCmd.CombinedOutput(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				log.WithFields(log.Fields{
					"experiment": sc.Service.Experiment(),
					"role":       sc.Service.Role(),
				}).Error("Context timeout")
				return ctx.Err()
			}
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Errorf("Error running condor_vault_storer to store and obtain tokens; %s", err.Error())
			log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
			}).Errorf("%s", stdoutStderr)
			return err
		} else {
			log.Infof("%s", stdoutStderr)
		}
	}

	log.WithFields(log.Fields{
		"experiment": sc.Service.Experiment(),
		"role":       sc.Service.Role(),
	}).Info("Successfully obtained and stored vault token")

	return nil
}

// validateVaultToken verifies that a vault token (service token as Hashicorp calls them) indicated by the filename is valid
func validateVaultToken(vaultTokenFilename string) error {
	vaultTokenBytes, err := ioutil.ReadFile(vaultTokenFilename)
	if err != nil {
		log.WithField("filename", vaultTokenFilename).Error("Could not read tokenfile for verification.")
		return err
	}

	vaultTokenString := string(vaultTokenBytes[:])

	if !IsServiceToken(vaultTokenString) {
		errString := "vault token failed validation"
		log.WithField("filename", vaultTokenFilename).Error(errString)
		return errors.New(errString)
	}

	return nil
}

// Borrowed from hashicorp's vault API, since we ONLY need this func
// Source: https://github.com/hashicorp/vault/blob/main/vault/version_store.go
// and https://github.com/hashicorp/vault/blob/main/sdk/helper/consts/token_consts.go

const (
	ServiceTokenPrefix       = "hvs."
	LegacyServiceTokenPrefix = "s."
)

func IsServiceToken(token string) bool {
	return strings.HasPrefix(token, ServiceTokenPrefix) ||
		strings.HasPrefix(token, LegacyServiceTokenPrefix)
}