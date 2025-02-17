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

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rifflock/lfshook"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/yukitsune/lokirus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"github.com/fermitools/managed-tokens/internal/contextStore"
	"github.com/fermitools/managed-tokens/internal/db"
	"github.com/fermitools/managed-tokens/internal/environment"
	"github.com/fermitools/managed-tokens/internal/metrics"
	"github.com/fermitools/managed-tokens/internal/notifications"
	"github.com/fermitools/managed-tokens/internal/tracing"
	"github.com/fermitools/managed-tokens/internal/utils"
)

var (
	currentExecutable       string
	buildTimestamp          string // Should be injected at build time with something like go build -ldflags="-X main.buildTimeStamp=$BUILDTIMESTAMP"
	version                 string // Should be injected at build time with something like go build -ldflags="-X main.version=$VERSION"
	exeLogger               *log.Entry
	notificationsDisabledBy disableNotificationsOption = DISABLED_BY_CONFIGURATION
)

var devEnvironmentLabel string

const devEnvironmentLabelDefault string = "production"

// Supported Timeouts and their defaults
var timeouts = map[string]time.Duration{
	"global":       time.Duration(300 * time.Second),
	"ferryrequest": time.Duration(30 * time.Second),
	"db":           time.Duration(10 * time.Second),
}

// Metrics
var (
	promDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "managed_tokens",
		Name:      "stage_duration_seconds",
		Help:      "The amount of time it took to run a stage of a Managed Tokens Service executable",
	},
		[]string{
			"executable",
			"stage",
		},
	)
	ferryRefreshTime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "managed_tokens",
			Name:      "last_ferry_refresh",
			Help:      "The timestamp of the last successful refresh of the username --> UID table from FERRY for the Managed Tokens Service",
		},
	)
)

var (
	startSetup   time.Time
	prometheusUp = true
)

var errExitOK = errors.New("exit 0")

func setup() error {
	startSetup = time.Now()

	// Get current executable name
	if exePath, err := os.Executable(); err != nil {
		log.Error("Could not get path of current executable")
	} else {
		currentExecutable = path.Base(exePath)
	}
	setupLogger := log.WithField("executable", currentExecutable)

	if err := utils.CheckRunningUserNotRoot(); err != nil {
		setupLogger.Error("Current user is root.  Please run this executable as a non-root user")
		return err
	}

	initFlags() // Parse our flags

	versionMessage := fmt.Sprintf("Managed tokens library version %s, build %s", version, buildTimestamp)
	if viper.GetBool("version") {
		fmt.Println(versionMessage)
		return errExitOK
	}
	setupLogger.Info(versionMessage)

	if err := initConfig(); err != nil {
		fmt.Println("Fatal error setting up configuration.  Exiting now")
		return err
	}

	// TODO Remove this after bug detailed in initFlags() is fixed upstream
	disableNotifyFlagWorkaround()
	// END TODO

	devEnvironmentLabel = getDevEnvironmentLabel()

	initLogs()
	if err := initTimeouts(); err != nil {
		setupLogger.Error("Fatal error setting up timeouts")
		return err
	}
	if err := initMetrics(); err != nil {
		setupLogger.Error("Error setting up metrics. Will still continue")
	}

	return nil
}

func initFlags() {
	// Defaults
	viper.SetDefault("notifications.admin_email", "fife-group@fnal.gov")

	// Flags
	pflag.String("admin", "", "Override the config file admin email")
	pflag.String("authmethod", "tls", "Choose method for authentication to FERRY.  Currently-supported choices are \"tls\" and \"jwt\"")
	pflag.StringP("configfile", "c", "", "Specify alternate config file")
	pflag.Bool("disable-notifications", false, "Do not send admin notifications")
	pflag.Bool("dont-notify", false, "Same as --disable-notifications")
	pflag.BoolP("test", "t", false, "Test mode.  Query FERRY, but do not make any database changes")
	pflag.BoolP("verbose", "v", false, "Turn on verbose mode")
	pflag.Bool("version", false, "Version of Managed Tokens library")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	// Aliases
	// TODO There's a possible bug in viper, where pflags don't get affected by registering aliases.  The following should work, at least for one alias:
	//  viper.RegisterAlias("dont-notify", "disableNotifications")
	//  viper.RegisterAlias("disable-notifications", "disableNotifications")
	// Instead, we have to work around this after we read in the config file (see setup())
}

// NOTE See initFlags().  This workaround will be removed when the possible viper bug referred to there is fixed.
func disableNotifyFlagWorkaround() {
	if viper.GetBool("disable-notifications") || viper.GetBool("dont-notify") {
		viper.Set("disableNotifications", true)
		notificationsDisabledBy = DISABLED_BY_FLAG
	}
}

func initConfig() error {
	// Get config file
	configFileName := "managedTokens"
	// Check for override
	if config := viper.GetString("configfile"); config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName(configFileName)
	}

	viper.AddConfigPath("/etc/managed-tokens/")
	viper.AddConfigPath("$HOME/.managed-tokens/")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.WithField("executable", currentExecutable).Errorf("Error reading in config file: %v", err)
		return err
	}

	return nil
}

// Set up logs
func initLogs() {
	log.SetLevel(log.DebugLevel)
	debugLogConfigLookup := "logs.refresh-uids-from-ferry.debugfile"
	logConfigLookup := "logs.refresh-uids-from-ferry.logfile"
	// Debug log
	log.AddHook(lfshook.NewHook(lfshook.PathMap{
		log.DebugLevel: viper.GetString(debugLogConfigLookup),
		log.InfoLevel:  viper.GetString(debugLogConfigLookup),
		log.WarnLevel:  viper.GetString(debugLogConfigLookup),
		log.ErrorLevel: viper.GetString(debugLogConfigLookup),
		log.FatalLevel: viper.GetString(debugLogConfigLookup),
		log.PanicLevel: viper.GetString(debugLogConfigLookup),
	}, &log.TextFormatter{FullTimestamp: true}))

	// Info log file
	log.AddHook(lfshook.NewHook(lfshook.PathMap{
		log.InfoLevel:  viper.GetString(logConfigLookup),
		log.WarnLevel:  viper.GetString(logConfigLookup),
		log.ErrorLevel: viper.GetString(logConfigLookup),
		log.FatalLevel: viper.GetString(logConfigLookup),
		log.PanicLevel: viper.GetString(logConfigLookup),
	}, &log.TextFormatter{FullTimestamp: true}))

	// Loki.  Example here taken from README: https://github.com/YuKitsune/lokirus/blob/main/README.md
	lokiOpts := lokirus.NewLokiHookOptions().
		// Grafana doesn't have a "panic" level, but it does have a "critical" level
		// https://grafana.com/docs/grafana/latest/explore/logs-integration/
		WithLevelMap(lokirus.LevelMap{log.PanicLevel: "critical"}).
		WithFormatter(&log.JSONFormatter{}).
		WithStaticLabels(lokirus.Labels{
			"app":         "managed-tokens",
			"command":     currentExecutable,
			"environment": devEnvironmentLabel,
		})
	lokiHook := lokirus.NewLokiHookWithOpts(
		viper.GetString("loki.host"),
		lokiOpts,
		log.InfoLevel,
		log.WarnLevel,
		log.ErrorLevel,
		log.FatalLevel)

	log.AddHook(lokiHook)

	exeLogger = log.WithField("executable", currentExecutable)
	exeLogger.Debugf("Using config file %s", viper.ConfigFileUsed())

	if viper.GetBool("test") {
		exeLogger.Info("Running in test mode")
	}
}

// Setup of timeouts, if they're set
func initTimeouts() error {
	// Save supported timeouts into timeouts map
	for timeoutKey, timeoutString := range viper.GetStringMapString("timeouts") {
		timeoutKey := strings.TrimSuffix(timeoutKey, "timeout")
		// Only save the timeout if it's supported, otherwise ignore it
		if _, ok := timeouts[timeoutKey]; ok {
			timeout, err := time.ParseDuration(timeoutString)
			if err != nil {
				exeLogger.WithField(timeoutKey, timeoutString).Warn("Could not parse configured timeout duration.  Using default")
				continue
			}
			timeouts[timeoutKey] = timeout
			exeLogger.WithField(timeoutKey, timeoutString).Debug("Configured timeout")
		}
	}

	// Verify that individual timeouts don't add to more than total timeout
	now := time.Now()
	timeForComponentCheck := now

	for timeoutKey, timeout := range timeouts {
		if timeoutKey != "global" {
			timeForComponentCheck = timeForComponentCheck.Add(timeout)
		}
	}

	timeForGlobalCheck := now.Add(timeouts["global"])
	if timeForComponentCheck.After(timeForGlobalCheck) {
		msg := "configured component timeouts exceed the total configured global timeout.  Please check all configured timeouts"
		exeLogger.Error(msg)
		return errors.New(msg)
	}
	return nil
}

// Set up prometheus metrics
func initMetrics() error {
	// Set up prometheus metrics
	if _, err := http.Get(viper.GetString("prometheus.host")); err != nil {
		exeLogger.Errorf("Error contacting prometheus pushgateway %s: %s.  The rest of prometheus operations will fail. "+
			"To limit error noise, "+
			"these failures at the experiment level will be registered as warnings in the log, "+
			"and not be sent in any notifications.", viper.GetString("prometheus.host"), err.Error())
		prometheusUp = false
		return err
	}

	metrics.MetricsRegistry.MustRegister(promDuration)
	metrics.MetricsRegistry.MustRegister(ferryRefreshTime)
	return nil
}

// initTracing initializes the tracing configuration and returns a function to shutdown the
// initialized TracerProvider and an error, if any.
func initTracing(ctx context.Context) (func(context.Context), error) {
	url := viper.GetString("tracing.url")
	if url == "" {
		msg := "no tracing url configured.  Continuing without tracing"
		exeLogger.Error(msg)
		return nil, errors.New(msg)
	}
	tp, shutdown, err := tracing.NewOTLPHTTPTraceProvider(ctx, url, devEnvironmentLabel)
	if err != nil {
		exeLogger.Error("could not obtain a TraceProvider.  Continuing without tracing")
		return nil, err
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{}) // In case any downstream services want to use this trace context
	return shutdown, nil
}

func main() {
	if err := setup(); err != nil {
		if errors.Is(err, errExitOK) {
			os.Exit(0)
		}
		log.Fatal("Error running setup actions.  Exiting")
	}

	// Global Context
	var globalTimeout time.Duration
	var ok bool

	if globalTimeout, ok = timeouts["global"]; !ok {
		exeLogger.Fatal("Could not obtain global timeout.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
	defer cancel()

	// Tracing has to be initialized here and not in setup because we need our global context to pass to child spans
	if tracingShutdown, err := initTracing(ctx); err == nil {
		defer tracingShutdown(ctx)
	}

	// Run our actual operation
	if err := run(ctx); err != nil {
		exeLogger.Fatal("Error running operations to update database from FERRY.  Exiting")
	}
	log.Debug("Finished run")
}

func run(ctx context.Context) error {
	// Order of operations:
	// 1. Open database to record FERRY data
	// 2. Set up admin notification emails
	// 3. a. Choose authentication method to FERRY
	// 4. b. Query FERRY for data
	// 5. Insert data into database
	// 6. Verify that INSERTed data matches response data from FERRY
	// 7. Push metrics and send necessary notifications

	ctx, span := otel.GetTracerProvider().Tracer("refresh-uids-from-ferry").Start(ctx, "refresh-uids-from-ferry")
	if viper.GetBool("test") {
		span.SetAttributes(attribute.KeyValue{Key: "test", Value: attribute.BoolValue(true)})
	}
	defer span.End()

	if viper.GetBool("disableNotifications") {
		exeLogger.Debugf("Notifications disabled by %s", notificationsDisabledBy.String())
	}

	var dbLocation string
	// Open connection to the SQLite database where UID info will be stored
	if viper.IsSet("dbLocation") {
		dbLocation = viper.GetString("dbLocation")
	} else {
		dbLocation = "/var/lib/managed-tokens/uid.db"
	}
	exeLogger.Debugf("Using db file at %s", dbLocation)

	database, err := db.OpenOrCreateDatabase(dbLocation)
	if err != nil {
		msg := "Could not open or create ManagedTokensDatabase"
		tracing.LogErrorWithTrace(span, exeLogger, msg)
		// Start up a notification manager JUST for the purpose of sending the email that we couldn't open the DB.
		// In the case of this executable, that's a fatal error and we should stop execution.
		if !viper.GetBool("disableNotifications") {
			admNotMgr, aReceiveChan, adminNotifications := setupAdminNotifications(ctx, nil)
			sendSetupErrorToAdminMgr(aReceiveChan, msg)
			close(aReceiveChan)

			// We need to pass the pointer in here since we need to pick up the
			// changes made to adminNotifications, as explained here:
			// https://stackoverflow.com/a/52070387
			// and mocked out here:
			// https://go.dev/play/p/rww0ORt94pU
			if err2 := sendAdminNotifications(ctx, admNotMgr, &adminNotifications); err2 != nil {
				msg := "error sending admin notifications"
				tracing.LogErrorWithTrace(span, exeLogger, msg)
				err := fmt.Errorf("%s regarding %w: %w", msg, err, err2)
				return err
			}
		}
		return fmt.Errorf("%s: %w", msg, err)
	}
	defer database.Close()
	span.AddEvent("Opened ManagedTokensDatabase")

	// Send admin notifications at end of run
	// Note:  We don't actually need the values of admNotMgr and adminNotifications beyond the next if-clause.  However, we will need
	// the value of aReceiveChan, and since setupAdminNotifications returns all three, we want to initialize all three, so they can possibly
	// be overridden in the next if-clause.  If they are not overridden, they will all stay nil, which is fine.
	var admNotMgr *notifications.AdminNotificationManager
	var aReceiveChan chan<- notifications.SourceNotification // Channel to send admin notifications on.  We need it here so we can close it at the end of the run
	var adminNotifications []notifications.SendMessager

	if !viper.GetBool("disableNotifications") {
		admNotMgr, aReceiveChan, adminNotifications = setupAdminNotifications(ctx, database)

		defer func() {
			// We don't check the error here, because we don't want to halt execution if the admin message can't be sent.  Just log it and move on
			close(aReceiveChan)
			sendAdminNotifications(ctx, admNotMgr, &adminNotifications)
		}()
	}

	// Send metrics anytime run() returns
	defer func() {
		if prometheusUp {
			if err := metrics.PushToPrometheus(viper.GetString("prometheus.host"), getPrometheusJobName()); err != nil {
				// Non-essential - don't halt execution here
				exeLogger.Error("Could not push metrics to prometheus pushgateway")
				return
			}
			exeLogger.Info("Finished pushing metrics to prometheus pushgateway")
		}
	}()

	// Setup complete
	if prometheusUp {
		promDuration.WithLabelValues(currentExecutable, "setup").Set(time.Since(startSetup).Seconds())
	}

	// Begin processing
	startRequest := time.Now()
	span.AddEvent("Starting Processing")

	// Add verbose to the global context
	if viper.GetBool("verbose") {
		ctx = contextStore.WithVerbose(ctx)
	}
	// Start up worker to aggregate all FERRY data
	ferryData := make([]db.FerryUIDDatum, 0)
	ferryDataChan := make(chan db.FerryUIDDatum) // Channel to send FERRY data from GetFERRYData worker to AggregateFERRYData worker
	aggFERRYDataDone := make(chan struct{})      // Channel to close when FERRY data aggregation is done
	go func(ferryDataChan <-chan db.FerryUIDDatum, aggFERRYDataDone chan<- struct{}) {
		defer close(aggFERRYDataDone)
		_, span := otel.GetTracerProvider().Tracer("refresh-uids-from-ferry").Start(ctx, "aggregate-ferry-data_anonFunc")
		defer span.End()
		for ferryDatum := range ferryDataChan {
			ferryData = append(ferryData, ferryDatum)
		}
	}(ferryDataChan, aggFERRYDataDone)

	usernames := getAllAccountsFromConfig()

	// Pick our authentication method
	var authFunc func() func(context.Context, string, string) (*http.Response, error)
	switch supportedFERRYAuthMethod(viper.GetString("authmethod")) {
	case tlsAuth:
		authFunc = withTLSAuth
		exeLogger.Debug("Using TLS to authenticate to FERRY")
	case jwtAuth:
		sc, err := newFERRYServiceConfigWithKerberosAuth(ctx)
		if err != nil {
			msg := "Could not create service config to authenticate to FERRY with a JWT. Exiting"
			if !viper.GetBool("disableNotifications") {
				sendSetupErrorToAdminMgr(aReceiveChan, msg)
			}
			exeLogger.Error(msg)
			os.Exit(1)
		}
		defer func() {
			prefix := environment.FILE.String()
			os.RemoveAll(strings.TrimPrefix(sc.GetValue(environment.Krb5ccname), prefix))
			exeLogger.Info("Cleared kerberos cache")
		}()
		authFunc = withKerberosJWTAuth(sc)
		exeLogger.Debug("Using JWT to authenticate to FERRY")
	default:
		msg := "unsupported authentication method to communicate with FERRY"
		tracing.LogErrorWithTrace(span, exeLogger, msg)
		return errors.New(msg)
	}
	span.SetAttributes(attribute.KeyValue{Key: "authmethod", Value: attribute.StringValue(viper.GetString("authmethod"))})

	// Start workers to get data from FERRY
	func() {
		span.AddEvent("Start FERRY data collection")
		ctx, span := otel.GetTracerProvider().Tracer("refresh-uids-from-ferry").Start(ctx, "get-ferry-data_anonFunc")
		defer span.End()
		ferryDataErrGroup := new(errgroup.Group) // errgroup.Group to make sure we finish all collection of FERRY data before closing the FERRY data channel
		defer close(ferryDataChan)

		ferryContext := ctx
		if timeout, ok := timeouts["ferryrequest"]; ok {
			ferryContext = contextStore.WithOverrideTimeout(ctx, timeout)
		}

		// For each username, query FERRY for UID info
		// Note: In Go 1.22, the weird behavior of closures running as goroutines was fixed, so that's reflected here.  See https://go.dev/doc/faq#closures_and_goroutines
		for _, username := range usernames {
			ferryDataErrGroup.Go(func() error {
				usernameCtx, span := otel.GetTracerProvider().Tracer("refresh-uids-from-ferry").Start(ferryContext, "get-ferry-data-per-username_anonFunc")
				span.SetAttributes(attribute.KeyValue{Key: "username", Value: attribute.StringValue(username)})
				defer span.End()
				return getAndAggregateFERRYData(usernameCtx, username, authFunc, ferryDataChan, aReceiveChan)
			})
		}
		// Don't close data channel until all workers have put their data in
		if err := ferryDataErrGroup.Wait(); err != nil {
			// It's OK if we have an error - just log it and move on so we can work with what data did come back
			msg := "Error getting FERRY data for one of the usernames.  Please investigate"
			tracing.LogErrorWithTrace(span, exeLogger, msg)
		}
	}()

	<-aggFERRYDataDone // Wait until FERRY data aggregation is done before we insert anything into DB
	promDuration.WithLabelValues(currentExecutable, "getFERRYData").Set(time.Since(startRequest).Seconds())
	span.AddEvent("End FERRY data collection")

	// If we got no data, that's a bad thing, since we always expect to be able to
	if len(ferryData) == 0 {
		msg := "no data collected from FERRY"
		if !viper.GetBool("disableNotifications") {
			sendSetupErrorToAdminMgr(aReceiveChan, msg)
		}
		tracing.LogErrorWithTrace(span, exeLogger, msg+". Exiting")
		return errors.New(msg)
	}

	// Stop here if we're in test mode
	if viper.GetBool("test") {
		exeLogger.Info("Finished gathering data from FERRY")

		ferryDataStringSlice := make([]string, 0, len(ferryData))
		for _, datum := range ferryData {
			ferryDataStringSlice = append(ferryDataStringSlice, datum.String())
		}
		exeLogger.Infof(strings.Join(ferryDataStringSlice, "; "))

		exeLogger.Info("Test mode finished")
		return nil
	}

	span.AddEvent("Start DB Update")
	// INSERT all collected FERRY data into FERRYUIDDatabase
	startDBInsert := time.Now()
	var dbContext context.Context
	if timeout, ok := timeouts["db"]; ok {
		dbContext = contextStore.WithOverrideTimeout(ctx, timeout)
	} else {
		dbContext = ctx
	}
	if err := database.InsertUidsIntoTableFromFERRY(dbContext, ferryData); err != nil {
		msg := "Could not insert FERRY data into database"
		if !viper.GetBool("disableNotifications") {
			sendSetupErrorToAdminMgr(aReceiveChan, msg)
		}
		tracing.LogErrorWithTrace(span, exeLogger, msg)
		return err
	}
	span.AddEvent("End DB Update")

	// Confirm and verify that INSERT was successful
	span.AddEvent("Start DB Verification")
	dbData, err := database.ConfirmUIDsInTable(ctx)
	if err != nil {
		msg := "Error running verification of INSERT"
		if !viper.GetBool("disableNotifications") {
			sendSetupErrorToAdminMgr(aReceiveChan, msg)
		}
		tracing.LogErrorWithTrace(span, exeLogger, msg)
		return err
	}

	if !checkFerryDataInDB(ferryData, dbData) {
		msg := "verification of INSERT failed.  Please check the logs"
		if !viper.GetBool("disableNotifications") {
			sendSetupErrorToAdminMgr(aReceiveChan, msg)
		}
		tracing.LogErrorWithTrace(span, exeLogger, msg)
		return errors.New(msg)
	}
	span.AddEvent("End DB Verification")

	exeLogger.Debug("Verified INSERT")
	tracing.LogSuccessWithTrace(span, exeLogger, "Successfully refreshed Managed Tokens DB")
	promDuration.WithLabelValues(currentExecutable, "refreshManagedTokensDB").Set(time.Since(startDBInsert).Seconds())
	ferryRefreshTime.SetToCurrentTime()
	return nil
}
