package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/fermitools/managed-tokens/internal/contextStore"
	"github.com/fermitools/managed-tokens/internal/environment"
	"github.com/fermitools/managed-tokens/internal/metrics"
	"github.com/fermitools/managed-tokens/internal/notifications"
	"github.com/fermitools/managed-tokens/internal/service"
	"github.com/fermitools/managed-tokens/internal/tracing"
	"github.com/fermitools/managed-tokens/internal/vaultToken"
)

var (
	tokenGetTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "managed_tokens",
			Name:      "last_token_get_timestamp",
			Help:      "The timestamp of the last successful obtaining of a service vault token that was not stored in a condor credd by the Managed Tokens Service",
		},
		[]string{
			"service",
		},
	)
	tokenGetDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "managed_tokens",
			Name:      "token_get_duration_seconds",
			Help:      "Duration (in seconds) for a vault token to be obtained",
		},
		[]string{
			"service",
		},
	)
	getFailureCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "managed_tokens",
		Name:      "failed_vault_token_get_count",
		Help:      "The number of times the Managed Tokens Service failed to get a vault token",
	},
		[]string{
			"service",
		},
	)
)

const getTokenDefaultTimeoutStr string = "60s"

func init() {
	metrics.MetricsRegistry.MustRegister(tokenGetTimestamp)
	metrics.MetricsRegistry.MustRegister(tokenGetDuration)
	metrics.MetricsRegistry.MustRegister(getFailureCount)

}

// getTokenSuccess is a type that conveys whether StoreAndGetTokenWorker successfully stores and obtains tokens for each service
type getTokenSuccess struct {
	service.Service
	success bool
}

// GetService returns the service associated with the getTokenSuccess object
func (v *getTokenSuccess) GetService() service.Service {
	return v.Service
}

// GetSuccess returns whether the token operation was successful
func (v *getTokenSuccess) GetSuccess() bool {
	return v.success
}

// getTokenWorker is a worker that listens for worker.Config objects on chans.GetServiceConfigChan(), and for the received objects,
// gets a vault token for the service defined in the worker.Config.  It returns when chans.GetServiceConfigChan() is closed,
// and it will in turn close the other chans in the passed in ChannelsForWorkers
func getTokenWorker(ctx context.Context, chans channelGroup) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.GetTokenWorker")
	defer span.End()

	// Don't close the NotificationsChan or SuccessChan until we're done sending notifications and success statuses
	defer func() {
		chans.closeWorkerSendChans()
		log.Debug("Closed GetTokenWorker Notifications and Success Chan")
	}()

	// Get timeout for getToken operations
	getTokenTimeout, defaultUsed, err := contextStore.GetProperTimeout(ctx, getTokenDefaultTimeoutStr)
	if err != nil {
		span.SetStatus(codes.Error, "Could not parse getToken timeout")
		log.Fatal("Could not parse getToken timeout")
	}
	if defaultUsed {
		log.Debug("Using default timeout for getToken")
	}

	// // For all the serviceConfigChans being sent in, get token
	for sc := range chans.serviceConfigChan {
		scLogger := log.WithField("service", sc.Service.Name())

		success := &getTokenSuccess{
			Service: sc.Service,
			success: true,
		}

		defer func(s *getTokenSuccess) {
			chans.successChan <- s
		}(success)

		getTokenTimeoutCtx, getTokenCancel := context.WithTimeout(ctx, getTokenTimeout)
		defer getTokenCancel()

		interactive, err := getInteractiveTokenGetterOptionFromConfig(*sc, GetToken)
		if err != nil {
			scLogger.Errorf("Could not get interactive token getter option from config. Assuming false: %s", err.Error())
			interactive = false
		}

		if interactive {
			scLogger.Debug("Using interactive token getter as per service config")
		}

		// Check the kind of TokenGetter to use. If there's no AlternateTokenGetterOption set, use the default token getter
		var useTokenGetter TokenGetter
		useTokenGetter = &tokenGetterConfig{
			vaultServer:   sc.VaultServer,
			tokenRootPath: sc.ServiceCreddVaultTokenPathRoot,
			serviceName:   sc.Service.Name(),
			interactive:   interactive,
			environ:       &sc.CommandEnvironment,
		} // Default

		if alternateTokenGetter, err := getAlternateTokenGetterOptionFromConfig(*sc, GetToken); err == nil && alternateTokenGetter != nil {
			useTokenGetter = alternateTokenGetter
			scLogger.Debug("Using alternate token getter from service config")
		}

		// Get the token
		if err = useTokenGetter.GetToken(getTokenTimeoutCtx); err != nil {
			// Send notification of error
			success.success = false

			// Check to see if we need to report a specific error
			var msg string
			var errToReport error
			if errors.Is(err, context.DeadlineExceeded) {
				msg = "timeout error"
				errToReport = fmt.Errorf("%s: %s", err, "timeout error")
			} else {
				msg = "could not store and get vault tokens"
				unwrappedErr := errors.Unwrap(err)
				errToReport = fmt.Errorf("%s: %s", msg, err.Error())
				if unwrappedErr != nil {
					// Check to see if authentication is needed.  This is an error condition for non-interactive token storing
					var authNeededErrorPtr *vaultToken.ErrAuthNeeded
					if errors.As(unwrappedErr, &authNeededErrorPtr) && !interactive {
						errToReport = fmt.Errorf("%s: %s", msg, unwrappedErr.Error())
					}
				}
			}
			chans.notificationsChan <- notifications.NewSetupError(errToReport.Error(), sc.Service.Name())
			tracing.LogErrorWithTrace(span, scLogger, msg)
			return
		}
		success.success = true
		tracing.LogSuccessWithTrace(span, scLogger, "Successfully got vault token")
	}
}

// TokenGetter is a type that can get vault tokens
type TokenGetter interface {
	GetToken(ctx context.Context) error
}

// tokenGetterConfig is a configuration type that contains the needed information for getting vault tokens
type tokenGetterConfig struct {
	vaultServer   string
	tokenRootPath string
	serviceName   string
	interactive   bool
	environ       *environment.CommandEnvironment
}

// GetToken gets a vault token for the serviceName defined in the tokenGetterConfig and stores it in the proper location.
// It will first try to use an existing vault token at the proper location (by default, t.tokenRootPath/vt_u<uid>_<serviceName>), and
// either leave that token undisturbed, or renew the vault token at that location.
// If that token does not exist, it will create a new vault token and install it at that location
// It will return an error if either a new token cannot be generated for some reason, or if the existing token cannot be renewed.
func (t *tokenGetterConfig) GetToken(ctx context.Context) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.StoreAndGetTokensForSchedd")
	span.SetAttributes(attribute.String("tokenRootPath", t.tokenRootPath))
	span.SetAttributes(attribute.String("service", t.serviceName))
	defer span.End()

	funcLogger := log.WithField("service", t.serviceName)

	start := time.Now()

	// If the vault token already exists, use it at its current location at vaultTokenPath. If it doesn't, create a temp file to use for getting the token,
	// and then move it to vaultTokenPath
	vaultTokenPath := getServiceTokenForCreddLocation(t.tokenRootPath, t.serviceName, "")
	useVTPath := vaultTokenPath
	if _, err := os.Stat(vaultTokenPath); errors.Is(err, fs.ErrNotExist) {
		// Vault token doesn't exist, so we need to make a new one and move it into place
		funcLogger.Debug("Vault token does not exist. Creating new temp file for vault token")
		tempVaultTokenFile, err := os.CreateTemp(os.TempDir(), "managed_tokens_vault_token_")
		if err != nil {
			tracing.LogErrorWithTrace(span, funcLogger, "Could not create temp file for vault token in getTokenWorker")
			return fmt.Errorf("could not create temp file for vault token: %w", err)
		}

		// If proper token obtaining and storage happens, this file actually won't exist by the time this defer runs.  That's fine -
		// we just want to make sure we clean up if something goes wrong
		defer os.Remove(tempVaultTokenFile.Name()) // nolint:errcheck
		useVTPath = tempVaultTokenFile.Name()
	}

	// Create a bearer token file location that we will throw away after getting the token
	_bearerTokenFile, err := os.CreateTemp(os.TempDir(), "managed_tokens_bearer_token_")
	if err != nil {
		// Send notification of error
		funcLogger.Error("Could not create temp file for bearer token in getToken worker")
	} else {
		defer func() {
			funcLogger.WithField("path", _bearerTokenFile.Name()).Debug("Removing bearer token temp file")
			os.Remove(_bearerTokenFile.Name()) // nolint:errcheck
		}()
	}

	verbose, err := contextStore.GetVerbose(ctx)
	if err != nil {
		funcLogger.Warn("Could not get verbose setting from context. Assuming false")
	}

	// Get token
	h, err := vaultToken.NewHtgettokenClient(t.vaultServer, useVTPath, _bearerTokenFile.Name(), t.environ)
	if err != nil {
		err2 := fmt.Errorf("could not create htgettoken client: %w", err)
		tracing.LogErrorWithTrace(span, funcLogger, err2.Error())
		return err2
	}
	if verbose {
		h = h.WithVerbose()
	}

	experiment, role := service.ExtractExperimentAndRoleFromServiceName(t.serviceName)
	if _, err = h.GetToken(ctx, experiment, role, t.interactive); err != nil {
		getFailureCount.WithLabelValues(t.serviceName).Inc()
		err2 := fmt.Errorf("could not get vault token: %w", err)
		tracing.LogErrorWithTrace(span, funcLogger, err2.Error())
		return err2
	}

	// Now move vault token into storage location if needed
	if useVTPath != vaultTokenPath {
		funcLogger.Debug("Moving new vault token into storage location")
		if err := moveFileCrossDevice(useVTPath, vaultTokenPath); err != nil {
			// If this fails, we want to still declare success. It just means that we will not have moved the token into
			// its proper storage location, and we will be recreating it next time
			tracing.LogErrorWithTrace(span, funcLogger, "Could not move vault token into storage location in getToken worker")
		}
		tracing.LogSuccessWithTrace(span, funcLogger, "Successfully moved vault token into storage location")
	}

	dur := time.Since(start).Seconds()
	tokenGetTimestamp.WithLabelValues(t.serviceName).SetToCurrentTime()
	tokenGetDuration.WithLabelValues(t.serviceName).Set(dur)

	span.SetStatus(codes.Ok, "Successfully obtained vault token for experiment/role")
	return nil
}
