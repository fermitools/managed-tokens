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

package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/fermitools/managed-tokens/internal/contextStore"
	"github.com/fermitools/managed-tokens/internal/metrics"
	"github.com/fermitools/managed-tokens/internal/notifications"
	"github.com/fermitools/managed-tokens/internal/service"
	"github.com/fermitools/managed-tokens/internal/tracing"
	"github.com/fermitools/managed-tokens/internal/vaultToken"
)

// Metrics
var (
	tokenStoreTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "managed_tokens",
			Name:      "last_token_store_timestamp",
			Help:      "The timestamp of the last successful store of a service vault token in a condor credd by the Managed Tokens Service",
		},
		[]string{
			"service",
			"credd",
		},
	)
	tokenStoreDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "managed_tokens",
			Name:      "token_store_duration_seconds",
			Help:      "Duration (in seconds) for a vault token to get stored in a condor credd",
		},
		[]string{
			"service",
			"credd",
		},
	)
	storeFailureCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "managed_tokens",
		Name:      "failed_vault_token_store_count",
		Help:      "The number of times the Managed Tokens Service failed to store a vault token in a condor credd",
	},
		[]string{
			"service",
			"credd",
		},
	)
)

const vaultStorerDefaultTimeoutStr string = "60s"

func init() {
	metrics.MetricsRegistry.MustRegister(tokenStoreTimestamp)
	metrics.MetricsRegistry.MustRegister(tokenStoreDuration)
	metrics.MetricsRegistry.MustRegister(storeFailureCount)

}

// vaultStorerSuccess is a type that conveys whether StoreAndGetTokenWorker successfully stores and obtains tokens for each service
type vaultStorerSuccess struct {
	service.Service
	success bool
}

func (v *vaultStorerSuccess) GetService() service.Service {
	return v.Service
}

func (v *vaultStorerSuccess) GetSuccess() bool {
	return v.success
}

// TODO Tests for both this worker func and the helper

// storeAndGetTokenWorker is a worker that listens on chans.GetServiceConfigChan(), and for the received worker.Config objects,
// stores a refresh token in the configured vault and obtains vault and bearer tokens.  It returns when chans.GetServiceConfigChan() is closed,
// and it will in turn close the other chans in the passed in ChannelsForWorkers
func storeAndGetTokenWorker(ctx context.Context, chans channelGroup) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.StoreAndGetTokenWorker")
	defer span.End()

	// Don't close the NotificationsChan or SuccessChan until we're done sending notifications and success statuses
	defer func() {
		chans.closeWorkerSendChans()
		log.Debug("Closed StoreAndGetTokenWorker Notifications and Success Chan")
	}()

	vaultStorerTimeout, defaultUsed, err := contextStore.GetProperTimeout(ctx, vaultStorerDefaultTimeoutStr)
	if err != nil {
		span.SetStatus(codes.Error, "Could not parse vault storer timeout")
		log.Fatal("Could not parse vault storer timeout")
	}
	if defaultUsed {
		log.Debug("Using default timeout for vault storer")
	}

	// Initialize cache for worker so we don't store the same token in the same credd more than once.
	cache := make(map[string]map[string]struct{})

	for sc := range chans.serviceConfigChan {
		func(sc *Config) {
			success := &vaultStorerSuccess{
				Service: sc.Service,
				success: true,
			}

			defer func(s *vaultStorerSuccess) {
				chans.successChan <- s
			}(success)

			configLogger := log.WithFields(log.Fields{
				"experiment": sc.Service.Experiment(),
				"role":       sc.Service.Role(),
				"service":    sc.Name(),
			})

			interactive, err := getInteractiveTokenGetterOptionFromConfig(*sc, StoreAndGetToken)
			if err != nil && !errors.Is(err, errNoWorkerTypeMapInConfig) {
				configLogger.Warn("Could not get interactive token getter option from config.  Using non-interactive token storer by default")
				interactive = false // Default to not using interactive token getter if there is any error getting the option from config
			}

			// Note that here, if the configuration option for caching tokens isn't set in the map, we'll get an errNoWorkerTypeMapInConfig error,
			// and in that case, we want to default to using the cache
			defaultUseCache := true
			useCache, err := getCachedTokenStorerOptionFromConfig(*sc, StoreAndGetToken)
			switch {
			case errors.Is(err, errNoWorkerTypeMapInConfig): // The default case here - nothing is set in the configuration
				useCache = defaultUseCache // No config option found for this, so use default
				configLogger.Debug("No cached token storer configuration found for this worker type.  Using cached token storer by default")
			case err != nil: // Here, we have some non-nil error that isn't errNoWorkerTypeMapInConfig, which means there was some other error
				// retrieving the option from the config.  Thus, we want to use the default
				useCache = defaultUseCache
				configLogger.Warn("Could not get cached token storer option from config.  Using cached token storer by default")
			}
			// If there's no error getting the config option for caching, then use whatever the config says

			if !useCache {
				configLogger.Info("Not using cache for token storing for this service")
			}

			errsToReport := make([]error, 0) // slice of errors we need to specifically highlight
			for _, schedd := range sc.Schedds {
				func(ctx context.Context, schedd string) {
					ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.StoreAndGetTokenWorker_anonFunc")
					span.SetAttributes(attribute.String("service", sc.ServiceNameFromExperimentAndRole()))
					span.SetAttributes(attribute.String("schedd", schedd))
					defer span.End()

					scheddLogger := configLogger.WithField("schedd", schedd)

					var useTokenStorerAndGetter TokenStorerAndGetter
					if alternateTokenStorerAndGetter, err := getAlternateTokenStorerAndGetterOptionFromConfig(*sc, StoreAndGetToken); err == nil && alternateTokenStorerAndGetter != nil {
						useTokenStorerAndGetter = alternateTokenStorerAndGetter
						scheddLogger.Debug("Using alternate token storer and getter from service config")
					} else {
						useTokenStorerAndGetter = vaultToken.NewVaultStorerClient(schedd, sc.VaultServer, &sc.CommandEnvironment)
					}

					// Wrap the TokenStorerAndGetter in a CachedTokenStorerAndGetter
					var c CachedTokenStorerAndGetter
					if !useCache {
						c = &noOpCachedTokenStorerAndGetter{useTokenStorerAndGetter}
					} else {
						_c := newCachedTokenStorerAndGetter(useTokenStorerAndGetter, cache) // Since cache is a map, it's passed by reference,
						// so any updates in the storeAndGetTokensForSchedd call will carry to the next iteration
						c = &_c
					}

					vaultStorerContext, vaultStorerCancel := context.WithTimeout(ctx, vaultStorerTimeout)
					defer vaultStorerCancel()

					if err := storeAndGetTokensForSchedd(
						vaultStorerContext,
						c,
						sc.Service.Name(),
						sc.ServiceCreddVaultTokenPathRoot,
						interactive); err != nil {
						success.success = false

						// Check to see if we need to report a specific error
						var msg string
						if errors.Is(err, context.DeadlineExceeded) {
							msg = "timeout error"
							errsToReport = append(errsToReport, fmt.Errorf("%s: %s", schedd, msg))
						} else {
							msg = "could not store and get vault tokens for schedd"
							unwrappedErr := errors.Unwrap(err)
							if unwrappedErr != nil {
								// Check to see if authorization is needed.  This is an error condition for non-interactive token storing
								var authNeededErrorPtr *vaultToken.ErrAuthNeeded
								if errors.As(unwrappedErr, &authNeededErrorPtr) {
									msg = fmt.Sprintf("%s: %s", msg, unwrappedErr.Error())
									errsToReport = append(errsToReport, fmt.Errorf("%s: %w", schedd, unwrappedErr))
								}
							}
						}
						tracing.LogErrorWithTrace(span, scheddLogger, msg)
						return
					}
					tracing.LogSuccessWithTrace(span, scheddLogger, "Successfully got and stored vault token for schedd")
				}(ctx, schedd)
			}

			if !success.success {
				msg := "Could not store and get vault tokens"
				for _, err := range errsToReport {
					msg = fmt.Sprintf("%s; %s", msg, err.Error())
				}
				tracing.LogErrorWithTrace(span, configLogger, msg)
				chans.notificationsChan <- notifications.NewSetupError(msg, sc.ServiceNameFromExperimentAndRole())
				return
			}
			tracing.LogSuccessWithTrace(span, configLogger, "Successfully got and stored vault tokens for all schedds")
		}(sc)
	}
}

// storeAndGetTokensForSchedd handles the process of staging, storing, and retrieving vault tokens
// for a given service and credd (credential daemon) combination. It performs the following steps:
//  1. Attempts to stage a previously stored token file, handling cases where no prior token exists
//     or where staging fails.
//  2. Ensures that any new token obtained is stored for future use, provided the operation succeeds.
//  3. Calls the provided TokenStorerAndGetter to obtain and store a new vault token, optionally
//     using interactive mode.
func storeAndGetTokensForSchedd(ctx context.Context, t CachedTokenStorerAndGetter, serviceName string, tokenRootPath string, interactive bool) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.StoreAndGetTokensForSchedd")
	span.SetAttributes(attribute.String("tokenRootPath", tokenRootPath))
	span.SetAttributes(attribute.String("service", serviceName))
	span.SetAttributes(attribute.String("credd", t.GetCredd()))
	defer span.End()

	var success = true

	funcLogger := log.WithFields(log.Fields{
		"service": serviceName,
		"credd":   t.GetCredd(),
	})
	start := time.Now()

	// Before we do anything, check to make sure our context hasn't already been canceled
	if ctx.Err() != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "context was canceled or the deadline exceeded before token vault staging.  Will not attempt to stage a stored token file or store vault token")
		success = false
		return ctx.Err()
	}

	// First, check our cache.  If it has the combo of t.GetCredd() and serviceName, we already have the token, so stop here
	if t.hasInCache(serviceName) {
		msg := "Token for service/credd combination already stored in cache for this worker run.  Skipping store and get operation."
		tracing.LogSuccessWithTrace(span, funcLogger, msg)
		return nil
	}

	// Stage prior vault token, if it exists
	restorePriorTokenFunc, err := backupCondorVaultToken(serviceName)
	if err != nil {
		funcLogger.Errorf("Error backing up current vault token at %s.  Will overwrite this with a new vault token.", vaultToken.GetCondorVaultTokenLocation(serviceName))
	}
	defer func() {
		if err := restorePriorTokenFunc(); err != nil {
			funcLogger.Errorf("Error restoring prior condor vault token.  Please see the logs to see where the token might have been backed up.")
		}
	}()

	if err = stageStoredTokenFile(tokenRootPath, serviceName, t.GetCredd()); err != nil {
		switch {
		case errors.Is(err, errNoServiceCreddToken):
			funcLogger.Info("No prior vault token exists for this service/credd combination.  Will get a new vault token")
		case errors.Is(err, errMoveServiceCreddToken):
			funcLogger.Warn("There was an error staging the prior vault token for this service and credd.  Will get a new vault token")
		default:
			tracing.LogErrorWithTrace(span, funcLogger, "Could not stage prior vault token.  Please investigate, and be aware that stale credentials may get stored.")
			success = false
		}
	}

	// Make sure we store whatever comes out of storing the vault token, if that is a successful operation.
	// Note that if this operation fails, assuming we got a condor vault token, that will
	// stick around unless something else cleans up vault tokens.  This is actually OK, since
	// eventually that vault token will expire, and htgettoken will determine that a new one is
	// needed.
	defer func() {
		if success {
			if err = storeServiceTokenForCreddFile(tokenRootPath, serviceName, t.GetCredd()); err != nil {
				funcLogger.Error("Could not store condor vault token for credd for future runs.  Please investigate")
			}
		}
	}()

	// Last context check again here:  before we store vault token, check to make sure our context hasn't already been canceled
	if ctx.Err() != nil {
		tracing.LogErrorWithTrace(span, funcLogger, "context was canceled or the deadline exceeded before token vault storage.  Will not attempt to store vault token")
		success = false
		return ctx.Err()
	}

	// Store vault token on credd
	if err := t.GetAndStoreToken(ctx, serviceName, interactive); err != nil {
		storeFailureCount.WithLabelValues(serviceName, t.GetCredd()).Inc()
		span.SetStatus(codes.Error, "could not store or validate vault token")
		return err
	}

	// Add to cache that we have stored this token for this credd/serviceName combo
	t.storeInCache(serviceName)

	dur := time.Since(start).Seconds()
	tokenStoreTimestamp.WithLabelValues(serviceName, t.GetCredd()).SetToCurrentTime()
	tokenStoreDuration.WithLabelValues(serviceName, t.GetCredd()).Set(dur)

	span.SetStatus(codes.Ok, "Successfully stored and obtained vault tokens for schedds")
	return nil
}

// TokenStorerAndGetter is a type that can get vault tokens and store them in a credd
type TokenStorerAndGetter interface {
	GetAndStoreToken(ctx context.Context, serviceName string, interactive bool) error
	GetCredd() string
	GetVaultServer() string
}

// CachedTokenStorerAndGetter is a TokenStorerAndGetter that also caches service/credd combinations
type CachedTokenStorerAndGetter interface {
	TokenStorerAndGetter
	storeInCache(serviceName string)
	hasInCache(serviceName string) bool
}

// cachedTokenStorerAndGetter is a struct that wraps a TokenStorerAndGetter and
// adds caching functionality to keep track of which service's tokens have been stored in a
// given credd during the current worker run.  Access to the cache is protected by a mutex to
// ensure thread safety when the provided storeInCache and hasInCache methods are used.
type cachedTokenStorerAndGetter struct {
	TokenStorerAndGetter
	// key is credd, value is serviceName:struct{}{} for fast lookup.
	// We're not using a sync.Map here since there is no concurrent access of this type
	cache map[string]map[string]struct{} // {credd: {serviceName: struct{}{}}}
	mux   sync.Mutex
}

// newCachedTokenStorerAndGetter returns a new cachedTokenStorerAndGetter that wraps a TokenStorerAndGetter and
// prepopulates the internal cache with an optional existing cache.
func newCachedTokenStorerAndGetter(t TokenStorerAndGetter, currentCache map[string]map[string]struct{}) cachedTokenStorerAndGetter {
	if currentCache == nil {
		currentCache = make(map[string]map[string]struct{})
	}
	if _, ok := currentCache[t.GetCredd()]; !ok {
		currentCache[t.GetCredd()] = make(map[string]struct{})
	}
	return cachedTokenStorerAndGetter{
		TokenStorerAndGetter: t,
		cache:                currentCache,
	}
}

// storeInCache adds the serviceName and credd combination to the cachedTokenStorerAndGetter cache
func (c *cachedTokenStorerAndGetter) storeInCache(serviceName string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	if _, ok := c.cache[c.GetCredd()]; !ok {
		c.cache[c.GetCredd()] = make(map[string]struct{})
	}
	c.cache[c.GetCredd()][serviceName] = struct{}{}
}

// hasInCache checks whether the serviceName and credd combination is in the cachedTokenStorerAndGetter cache
func (c *cachedTokenStorerAndGetter) hasInCache(serviceName string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()

	if _, ok := c.cache[c.GetCredd()]; !ok {
		return false
	}
	if _, ok := c.cache[c.GetCredd()][serviceName]; !ok {
		return false
	}
	return true
}

// noOpCachedTokenStorerAndGetter is a struct that implements the CachedTokenStorerAndGetter interface
// but does not actually cache anything.
type noOpCachedTokenStorerAndGetter struct {
	TokenStorerAndGetter
}

func (n *noOpCachedTokenStorerAndGetter) storeInCache(serviceName string)    {} // No op
func (n *noOpCachedTokenStorerAndGetter) hasInCache(serviceName string) bool { return false }
