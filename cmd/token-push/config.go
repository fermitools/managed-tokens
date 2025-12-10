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

package main

// config.go provides utilities that are meant to be used by token-push to interact with the configuration

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/fermitools/managed-tokens/internal/environment"
	"github.com/fermitools/managed-tokens/internal/service"
	"github.com/fermitools/managed-tokens/internal/worker"
)

var (
	logHtGettokenOptsOnce sync.Once   // Only log our environment's HTGETTOKENOPTS once
	globalScheddCache     scheddCache // Global cache for the schedds, sorted by collector host
)

func init() {
	globalScheddCache.cache = make(map[string]*scheddCacheEntry)
	globalScheddCache.mu = &sync.Mutex{}
}

// Functional options for initialization of service Config

// getUserPrincipalFromConfiguration gets the configured kerberos principal
func getUserPrincipalFromConfiguration(configPath string) string {
	if userPrincipalOverrideConfigPath, ok := getConfigOverridePath(configPath, "userPrincipal"); ok {
		return viper.GetString(userPrincipalOverrideConfigPath)
	} else {
		kerberosPrincipalPattern, _ := getConfigOverridePath(configPath, "kerberosPrincipalPattern")
		userPrincipalTemplate, err := template.New("userPrincipal").Parse(viper.GetString(kerberosPrincipalPattern))
		if err != nil {
			log.Errorf("Error parsing Kerberos Principal Template, %s", err)
			return ""
		}
		account := viper.GetString(configPath + ".account")
		templateArgs := struct{ Account string }{Account: account}

		var b strings.Builder
		if err := userPrincipalTemplate.Execute(&b, templateArgs); err != nil {
			log.WithField("account", account).Error("Could not execute kerberos prinicpal template")
			return ""
		}
		return b.String()
	}
}

// getUserPrincipalAndHtgettokenoptsFromConfiguration gets a worker.Config's kerberos principal and with it, the value for the HTGETTOKENOPTS environment variable
func getUserPrincipalAndHtgettokenoptsFromConfiguration(configPath string) (userPrincipal string, htgettokenOpts string) {
	htgettokenOptsPtr := &htgettokenOpts
	defer func() {
		if htgettokenOptsPtr != nil {
			log.Debugf("Final HTGETTOKENOPTS: %s", *htgettokenOptsPtr)
		}
	}()

	userPrincipal = getUserPrincipalFromConfiguration(configPath)
	if userPrincipal == "" {
		log.WithField("caller", "setUserPrincipalAndHtgettokenopts").Error("User principal is blank.  Cannot determine credkey and thus HTGETTOKENOPTS.")
		return
	}

	credKey := strings.ReplaceAll(userPrincipal, "@FNAL.GOV", "")

	// Look for HTGETTOKKENOPTS in environment.  If it's given here, take as is, but add credkey if it's absent
	if viper.IsSet("ORIG_HTGETTOKENOPTS") {
		htgettokenOpts = resolveHtgettokenOptsFromConfig(credKey)
		return
	}

	// HTGETTOKENOPTS was not in the environment.  Use our defaults.
	// Calculate minimum vault token lifetime from config
	lifetimeString := getTokenLifetimeStringFromConfiguration()
	htgettokenOpts = "--vaulttokenminttl=" + lifetimeString + " --credkey=" + credKey

	return
}

// resolveHtgettokenOptsFromConfig checks the config for the "ORIG_HTGETTOKENOPTS" key.  If that is set, check the ORIG_HTGETTOKENOPTS value for the
// given credKey.  If the credKey is present, return the ORIG_HTGETTOKENOPTS value.  Otherwise, return the ORIG_HTGETTOKENOPTS value with the credKey
// appended
func resolveHtgettokenOptsFromConfig(credKey string) string {
	origHtgettokenOpts := viper.GetString("ORIG_HTGETTOKENOPTS")

	// ORIG_HTGETTOKENOPTS not set in config
	if origHtgettokenOpts == "" {
		return "--credkey=" + credKey
	}

	log.Debugf("Prior to running, HTGETTOKENOPTS was set to %s", origHtgettokenOpts)
	// If we have the right credkey in the HTGETTOKENOPTS, leave it be
	if strings.Contains(origHtgettokenOpts, credKey) {
		return origHtgettokenOpts
	}
	logHtGettokenOptsOnce.Do(
		func() {
			log.Warn("HTGETTOKENOPTS was provided in the environment and does not have the proper --credkey specified.  Will add it to the existing HTGETTOKENOPTS")
		},
	)
	htgettokenOpts := origHtgettokenOpts + " --credkey=" + credKey
	return htgettokenOpts
}

// getTokenLifetimeStringFromConfiguration checks the configuration for the "minTokenLifetime" key.  If it is set, the value is returned.  Otherwise,
// a default is returned.
func getTokenLifetimeStringFromConfiguration() string {
	defaultLifetimeString := "10s"
	if viper.IsSet("minTokenLifetime") {
		return viper.GetString("minTokenLifetime")
	}
	return defaultLifetimeString
}

// getKeytabFromConfiguration checks the configuration at the configPath for an override for the path to the kerberos keytab.
// If the override does not exist, it uses the configuration to calculate the default path to the keytab
func getKeytabFromConfiguration(configPath string) string {
	if keytabConfigPath, ok := getConfigOverridePath(configPath, "keytabPath"); ok {
		return viper.GetString(keytabConfigPath)
	} else {
		// Default keytab location
		return path.Join(
			viper.GetString(keytabConfigPath),
			fmt.Sprintf(
				"%s.keytab",
				viper.GetString(configPath+".account"),
			),
		)
	}
}

// getScheddsAndCollectorHostFromConfiguration gets the schedd names that match the configured constraint by querying the condor collector.  It can be overridden
// by setting the configPath's condorCreddHostOverride field, in which case that value will be set as the schedd. It returns
// the collector host used, and the list of schedds that were found.  If no valid collector host or schedds are found, an error is returned.
func getScheddsAndCollectorHostFromConfiguration(ctx context.Context, configPath string) (string, []string, error) {
	funcLogger := log.WithField("configPath", configPath)
	ctx, span := otel.Tracer("managed-tokens").Start(ctx, "getScheddsFromConfiguration")
	span.SetAttributes(attribute.KeyValue{Key: "configPath", Value: attribute.StringValue(configPath)})
	defer span.End()

	collectorHostString := getCondorCollectorHostFromConfiguration(configPath)
	if collectorHostString == "" {
		msg := "no collector hosts found"
		span.SetStatus(codes.Error, msg)
		funcLogger.Error(msg)
		return "", nil, errors.New(msg)
	}
	collectorHostEntries := strings.Split(collectorHostString, ",")

	// 1. Try override
	// If condorCreddHostOverride is set either globally or at service level, set the schedd slice to that, and return first collector host
	schedds, found := checkScheddsOverride(configPath)
	if found {
		span.SetStatus(codes.Ok, "Schedds successfully retrieved from override")
		span.SetAttributes(
			attribute.KeyValue{Key: "scheddInfoSource", Value: attribute.StringValue("override")},
			attribute.KeyValue{Key: "schedds", Value: attribute.StringValue(strings.Join(schedds, ","))},
		)
		return collectorHostEntries[0], schedds, nil
	}

	// Acquire our lock for the globalScheddCache at this point
	globalScheddCache.mu.Lock()
	defer globalScheddCache.mu.Unlock()

	// 2.  Try globalScheddCache
	for _, collectorHostEntry := range collectorHostEntries {
		// See if we already have created a cacheEntry in the globalScheddCache for the collectorHost
		scheddSourceForLog := "cache"

		var cacheEntry *scheddCacheEntry
		cacheEntry, ok := globalScheddCache.cache[collectorHostEntry]
		if !ok {
			// New cache entry
			cacheEntry = &scheddCacheEntry{
				newScheddCollection(),
				&sync.Once{},
				nil,
			}
			globalScheddCache.cache[collectorHostEntry] = cacheEntry
		}

		// Now that we have our *scheddCacheEntry (either new or preexisting), if its *sync.Once has not been run, do so now to populate the entry.
		// If the Once has already been run, it will wait until the first Once has completed before resuming execution.
		// This way we are guaranteed that the cache will always be populated.
		cacheEntry.once.Do(
			func() {
				// 3.  Query collector
				// At this point, we haven't queried this collector yet.  Do so, and store its schedds in the global store/cache
				ctx, span := otel.Tracer("managed-tokens").Start(ctx, "getScheddsFromConfiguration.queryCollAnonFunc")
				span.SetAttributes(attribute.KeyValue{Key: "collectorHost", Value: attribute.StringValue(collectorHostEntry)})
				defer span.End()

				scheddSourceForLog = "collector"
				constraint := getConstraintFromConfiguration(configPath)
				cacheEntry.err = cacheEntry.populateFromCollector(ctx, collectorHostEntry, constraint)
				if cacheEntry.err != nil {
					span.SetStatus(codes.Error, "Could not populate schedd cache from collector")
				}
			},
		)
		if cacheEntry.err != nil {
			// Move to the next collectorHostEntry because we either couldn't populate a cacheEntry, or we already have an error
			// from a previous attempt to populate it
			if ok {
				// We didn't try to query the collector because a previous attempt failed.  Indicate this in the log
				funcLogger.WithField("collectorHost", collectorHostEntry).Debug("Previous attempt to query this collector failed.  Skipping.")
			}
			continue
		}

		// Load schedds from cache, which we either just populated, or are only reading from; then return those schedds
		schedds = cacheEntry.scheddCollection.getSchedds()
		funcLogger.WithFields(log.Fields{
			"schedds":       schedds,
			"collectorHost": collectorHostEntry,
		}).Debugf("Set schedds successfully from %s", scheddSourceForLog)
		span.SetAttributes(attribute.KeyValue{Key: "scheddInfoSource", Value: attribute.StringValue(scheddSourceForLog)})
		span.SetStatus(codes.Ok, "Schedds successfully retrieved")
		return collectorHostEntry, schedds, nil
	}

	// All attempts to get schedds have failed
	msg := "could not retrieve schedds from configured collectors"
	span.SetStatus(codes.Error, msg)
	return "", nil, errors.New(msg)
}

// getCondorCollectorHostFromConfiguration gets the condor collector host from the Viper configuration which will be used to populate
// the _condor_COLLECTOR_HOST environment variable.
// It is preferred to use getScheddsAndCollectorHostFromConfiguration to get the collector host, as it will also populate the schedd cache
// and handle failovers
func getCondorCollectorHostFromConfiguration(configPath string) string {
	condorCollectorHostPath, _ := getConfigOverridePath(configPath, "condorCollectorHost")
	return viper.GetString(condorCollectorHostPath)
}

// checkScheddsOverride checks the global and service-level configurations for the condorCreddHost key.  If that key exists, the value
// is returned, along with a bool indicating that the key was found in the configuration.
func checkScheddsOverride(configPath string) (schedds []string, found bool) {
	creddOverrideVar, _ := getConfigOverridePath(configPath, "condorCreddHost")
	if viper.IsSet(creddOverrideVar) {
		schedds = append(schedds, viper.GetString(creddOverrideVar))
		log.WithFields(log.Fields{
			"configPath": configPath,
			"schedds":    schedds,
		}).Debugf("Set schedds successfully from override")
		return schedds, true
	}
	return
}

// getConstraintFromConfiguration checks the configuration at the configPath for an override for the path to a condor constraint
// If the override does not exist, it returns the globally-configured condor constraint.
func getConstraintFromConfiguration(configPath string) string {
	var constraint string
	constraintKey, _ := getConfigOverridePath(configPath, "condorScheddConstraint")
	if viper.IsSet(constraintKey) {
		constraint = viper.GetString(constraintKey)
		log.WithField("constraint", constraint).Debug("Found constraint for condor collector query (condor_status)")
	}
	return constraint
}

// getVaultServer queries various sources to get the correct vault server or SEC_CREDENTIAL_GETTOKEN_OPTS setting, which condor_vault_storer
// needs to store the refresh token in a vault server.  The order of precedence is:
//
// 1. Environment variable _condor_SEC_CREDENTIAL_GETTOKEN_OPTS
// 2. Configuration file for managed tokens
// 3. Condor configuration file SEC_CREDENTIAL_GETTOKEN_OPTS value
func getVaultServer(configPath string) (string, error) {
	// Check environment
	if val := os.Getenv(environment.CondorSecCredentialGettokenOpts.EnvVarKey()); val != "" {
		return parseVaultServerFromEnvSetting(val)
	}

	// Check config
	if vaultServerConfigKey, _ := getConfigOverridePath(configPath, "vaultServer"); viper.IsSet(vaultServerConfigKey) {
		return viper.GetString(vaultServerConfigKey), nil
	}

	// Then check condor
	if val, err := getSecCredentialGettokenOptsFromCondor(); err != nil {
		log.Error("Could not get SEC_CREDENTIAL_GETTOKEN_OPTS from HTCondor")
	} else {
		return parseVaultServerFromEnvSetting(val)
	}

	return "", errors.New("could not find setting for SEC_CREDENTIAL_GETTOKEN_OPTS in environment, configuration, or HTCondor")
}

// getSecCredentialGettokenOptsFromCondor checks the condor configuration for the SEC_CREDENTIAL_GETTOKEN_OPTS setting
// and if available, returns it
func getSecCredentialGettokenOptsFromCondor() (string, error) {
	condorVarName := environment.CondorSecCredentialGettokenOpts.EnvVarKey()
	varName := strings.TrimPrefix(condorVarName, "_condor_")

	cmd := exec.Command("condor_config_val", varName)
	out, err := cmd.Output()
	if err != nil {
		log.Errorf("Could not run condor_config_val to get SEC_CREDENTIAL_GETTOKEN_OPTS: %s", err)
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getServiceCreddVaultTokenPathRoot checks the configuration at the configPath for an override for the path to the directory
// where the condorVaultStorer worker should look for and store service/credd-specific vault tokens.  If the override does not exist,
// it uses the configuration to calculate the default path to the relevant directory
func getServiceCreddVaultTokenPathRoot(configPath string) string {
	serviceCreddVaultTokenPathRootPath, _ := getConfigOverridePath(configPath, "serviceCreddVaultTokenPathRoot")
	return viper.GetString(serviceCreddVaultTokenPathRootPath)
}

// getFileCopierOptionsFromConfig gets the fileCopierOptions from the configuration.  If fileCopierOptions
// is overridden at the service configuration level, then the global configuration value is ignored.
func getFileCopierOptionsFromConfig(configPath string) []string {
	fileCopierOptsPath, _ := getConfigOverridePath(configPath, "fileCopierOptions")
	fileCopierOptsString := viper.GetString(fileCopierOptsPath)
	fileCopierOpts, _ := shlex.Split(fileCopierOptsString)
	return fileCopierOpts
}

// getPingOptsFromConfig checks the configuration at the configPath for an override for
// extra args to pass to the ping worker.  If the override does not exist,
// it uses the configuration to calculate the default path to the relevant directory
func getPingOptsFromConfig(configPath string) []string {
	pingOptsPath, _ := getConfigOverridePath(configPath, "pingOptions")
	pingOptsString := viper.GetString(pingOptsPath)
	pingOpts, _ := shlex.Split(pingOptsString)
	return pingOpts
}

// getSSHOptsFromConfig checks the configuration at the configPath for an override for
// extra args to pass to the fileCopier worker.  If the override does not exist,
// it uses the configuration to calculate the default path to the relevant directory
func getSSHOptsFromConfig(configPath string) []string {
	sshOptsPath, _ := getConfigOverridePath(configPath, "sshOptions")
	sshOptsString := viper.GetString(sshOptsPath)
	sshOpts, _ := shlex.Split(sshOptsString)
	return sshOpts
}

// getDefaultRoleFileDestinationTemplate gets the template that the pushTokenWorker should use when
// deriving the default role file path on the destination node.
func getDefaultRoleFileDestinationTemplate(configPath string) string {
	defaultRoleFileDestinationTmplPath, _ := getConfigOverridePath(configPath, "defaultRoleFileDestinationTemplate")
	if !viper.IsSet(defaultRoleFileDestinationTmplPath) {
		return "/tmp/default_role_{{.Experiment}}_{{.DesiredUID}}" // Default role file destination template
	}
	return viper.GetString(defaultRoleFileDestinationTmplPath)
}

// tokenGetterWorkerType is an enum representing the type of token getter worker to use. It is mainly used where
// restriction is needed of a worker type
//
// Note: This restriction could have been done with an interface, but that was deemed to be too complicated.
type tokenGetterWorkerType uint8

const (
	storeAndGetTokenWorkerType tokenGetterWorkerType = iota
	storeAndGetTokenInteractiveWorkerType
	getTokenWorkerType
	invalidTokenGetterWorkerType
)

func isValidTokenGetterWorkerType(t tokenGetterWorkerType) bool {
	return t < invalidTokenGetterWorkerType
}

// workerTypeToTokenGetterWorkerType converts a worker.WorkerType to a tokenGetterWorkerType if possible
func workerTypeToTokenGetterWorkerType(wt worker.WorkerType) tokenGetterWorkerType {
	switch wt {
	case worker.StoreAndGetTokenWorkerType:
		return storeAndGetTokenWorkerType
	case worker.StoreAndGetTokenInteractiveWorkerType:
		return storeAndGetTokenInteractiveWorkerType
	case worker.GetTokenWorkerType:
		return getTokenWorkerType
	default:
		return invalidTokenGetterWorkerType
	}
}

func tokenGetterWorkerTypeToWorkerType(t tokenGetterWorkerType) (worker.WorkerType, error) {
	switch t {
	case storeAndGetTokenWorkerType:
		return worker.StoreAndGetTokenWorkerType, nil
	case storeAndGetTokenInteractiveWorkerType:
		return worker.StoreAndGetTokenInteractiveWorkerType, nil
	case getTokenWorkerType:
		return worker.GetTokenWorkerType, nil
	default:
		return 0, errors.New("invalid tokenGetterWorkerType")
	}
}

// getTokenGetterOverrideFromConfiguration checks the configuration for an overridden tokenGetterWorkerType.
// If the override key "<configPath>.tokenGetterOverride" exists, the function validates the value, and returns
// the corresponding tokenGetterWorkerType. If validation fails, or the override key is not set in the configuration,
// the default of storeAndGetTokenWorkerType is returned.
func getTokenGetterOverrideFromConfiguration(configPath string) tokenGetterWorkerType {
	if tokenGetterOverridePath, overridden := getConfigOverridePath(configPath, "tokenGetter"); overridden {
		// Check the configuration value against the possible valid worker type configuration strings
		overrideValue := viper.GetString(tokenGetterOverridePath)
		overrideWorkerType, ok := workerTypeFromConfig(overrideValue)
		if !ok {
			log.Errorf("Invalid tokenGetter override value %s found in configuration at %s. Using default", overrideValue, tokenGetterOverridePath)
			return storeAndGetTokenWorkerType
		}

		// Then make sure that's a tokenGetterWorkerType, and return it if it is
		overrideTokenGetterWorkerType := workerTypeToTokenGetterWorkerType(overrideWorkerType)
		if !isValidTokenGetterWorkerType(overrideTokenGetterWorkerType) {
			log.Errorf("Invalid tokenGetter override worker type %s found in configuration at %s. Using default", overrideValue, tokenGetterOverridePath)
			return storeAndGetTokenWorkerType
		}
		log.Infof("Using tokenGetter override from configuration: %s", overrideWorkerType.String())
		return overrideTokenGetterWorkerType // Use validated override value
	}
	log.Debug("Using default tokenGetter override from configuration")
	return storeAndGetTokenWorkerType // Default
}

// resolveDisableNotifications checks each service's configuration to determine if notifications should be disabled.
// It takes a slice of service objects as input and returns a boolean indicating whether admin notifications should be disabled,
// and a slice of strings containing the names of services for which notifications should be disabled.
func resolveDisableNotifications(services []service.Service) (bool, []string) {
	serviceNotificationsToDisable := make([]string, 0, len(services))
	globalDisableNotifications := viper.GetBool("disableNotifications")
	finalDisableAdminNotifications := globalDisableNotifications

	// Check each service's override
	for _, s := range services {
		serviceConfigPath := "experiments." + s.Experiment() + ".roles." + s.Role()
		disableNotificationsPath, _ := getConfigOverridePath(serviceConfigPath, "disableNotifications")
		serviceDisableNotifications := viper.GetBool(disableNotificationsPath)

		// If global setting is to disable notifications (true), but any one of the experiments wants to have notifications sent (false),
		// we need to send admin notifications for that service too, so override the global setting
		if (!serviceDisableNotifications) && globalDisableNotifications {
			finalDisableAdminNotifications = false
		}

		// If the service wants to disable notifications, either through override or from the global setting, add it to the list
		if serviceDisableNotifications {
			serviceNotificationsToDisable = append(serviceNotificationsToDisable, getServiceName(s))
		}
	}

	return finalDisableAdminNotifications, serviceNotificationsToDisable
}

// Utility functions

// getConfigOverridePath checks to see if key + "Override" is defined at the checkConfigPath in the configuration.
// If so, the full configuration path is returned, and the overridden bool is set to true.
// If not, the original key is returned, and the overridden bool is set to false
func getConfigOverridePath(checkConfigPath, key string) (configPath string, overridden bool) {
	configPath = key
	overrideConfigPath := checkConfigPath + "." + key + "Override"
	if viper.IsSet(overrideConfigPath) {
		return overrideConfigPath, true
	}
	return
}

// parseVaultServerFromEnvSetting takes an environment setting meant for htgettoken to parse (for example "-a vaultserver.domain"),
// and returns the vaultServer from that setting (in the above example, "vaultserver.domain", nil would be returned)
func parseVaultServerFromEnvSetting(envSetting string) (string, error) {
	envSettingArgs, err := shlex.Split(envSetting)
	if err != nil {
		log.Errorf("Could not split environment setting according to shlex rules, %s", err)
		return "", err
	}

	envSettingFlagSet := pflag.NewFlagSet("envSetting", pflag.ContinueOnError)
	envSettingFlagSet.ParseErrorsWhitelist.UnknownFlags = true // We're ok with unknown flags - just skip them
	var vaultServerPtr *string = envSettingFlagSet.StringP("vaultserver", "a", "", "")
	envSettingFlagSet.Parse(envSettingArgs)

	noVaultServerErr := errors.New("no vault server was stored in the environment")

	if vaultServerPtr == nil {
		return "", noVaultServerErr
	}
	if *vaultServerPtr == "" {
		return "", noVaultServerErr
	}

	return *vaultServerPtr, nil
}

// createWorkerRetryMap creates a map of worker.WorkerTypes to their retry configuration.
// It validates each set of retry count/sleep duration values against the total
// configured timeout for that WorkerType before adding them to the map.
func createWorkerRetryMap(timeoutsMap map[timeoutKey]time.Duration) (map[worker.WorkerType]workerRetryConfig, error) {
	workerRetryMap := make(map[worker.WorkerType]workerRetryConfig)

	_retryArgs := []struct {
		worker.WorkerType
		// The timeout that we should be validating before using it
		checkTimeout time.Duration
	}{
		{worker.GetKerberosTicketsWorkerType, timeoutsMap[timeoutKerberos]},
		{worker.StoreAndGetTokenWorkerType, timeoutsMap[timeoutVaultStorer]},
		{worker.StoreAndGetTokenInteractiveWorkerType, timeoutsMap[timeoutVaultStorer]},
		{worker.PingAggregatorWorkerType, timeoutsMap[timeoutPing]},
		{worker.PushTokensWorkerType, timeoutsMap[timeoutPush]},
	}
	for _, retryArg := range _retryArgs {
		numRetries, retrySleep, err := getAndCheckRetryInfoFromConfig(retryArg.WorkerType, retryArg.checkTimeout)
		if err != nil {
			msg := fmt.Sprintf("invalid timeout %s: %s", workerTypeToConfigString(retryArg.WorkerType), err.Error())
			return nil, errors.New(msg)
		}
		workerRetryMap[retryArg.WorkerType] = workerRetryConfig{
			numRetries: uint(numRetries),
			retrySleep: retrySleep,
		}
	}
	return workerRetryMap, nil
}

func checkRetryTimeout(numRetries int, retrySleepDuration time.Duration, timeout time.Duration) error {
	if timeout < time.Duration(numRetries)*retrySleepDuration {
		return fmt.Errorf("timeout (%s) is less than numRetries*retrySleepDuration (%s)", timeout, time.Duration(numRetries)*retrySleepDuration)
	}
	return nil
}

func setDefaultWorkerRetryMap() map[worker.WorkerType]workerRetryConfig {
	m := make(map[worker.WorkerType]workerRetryConfig)
	for _, wt := range validWorkerTypes {
		// Check to make sure our worker type is one that supports retries
		for validWt := range worker.ValidRetryWorkerTypes() {
			if wt != validWt {
				continue
			}
			m[wt] = workerRetryConfig{
				numRetries: 0,
				retrySleep: time.Duration(0 * time.Second),
			}
		}
	}

	return m
}

// getAndCheckRetryInfoFromConfig gets the number of retries and the sleep time between retries from the configuration
// for a particular worker type key in the configuration.  It then checks that the retry timeout is less than the
// given duration.
func getAndCheckRetryInfoFromConfig(wt worker.WorkerType, checkTimeout time.Duration) (numRetries int, retrySleep time.Duration, err error) {
	numRetries = getWorkerConfigInteger[int](wt, "numRetries")
	retrySleep = getWorkerConfigTimeDuration(wt, "retrySleep")
	if err := checkRetryTimeout(numRetries, retrySleep, checkTimeout); err != nil {
		msg := "timeout is less than the time it would take to retry all attempts.  Will stop now"
		return 0, 0, errors.New(msg)
	}
	return numRetries, retrySleep, nil
}
