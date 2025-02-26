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
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/fermitools/managed-tokens/internal/db"
	"github.com/fermitools/managed-tokens/internal/worker"
)

// Functional option helpers for worker.Config initialization

// getDesiredUIDByOverrideOrLookup gets the DesiredUID for a service by checking the configuration in the "account"
// field.  It then checks the configuration to see if there is a configured override for the UID.  If it not overridden,
// the default behavior is to query the managed tokens database that should be populated by the refresh-uids-from-ferry executable.
//
// If the default behavior is not possible, the configuration should have a desiredUIDOverride field to allow token-push to run properly
func getDesiredUIDByOverrideOrLookup(ctx context.Context, serviceConfigPath string, database *db.ManagedTokensDatabase) (uint32, error) {
	ctx, span := otel.GetTracerProvider().Tracer("token-push").Start(ctx, "getDesiredUIDByOverrideOrLookup")
	span.SetAttributes(attribute.KeyValue{Key: "serviceConfigPath", Value: attribute.StringValue(serviceConfigPath)})
	defer span.End()

	if viper.IsSet(serviceConfigPath + ".desiredUIDOverride") {
		return viper.GetUint32(serviceConfigPath + ".desiredUIDOverride"), nil
	}
	// Get UID from SQLite DB that should be kept up to date by refresh-uids-from-ferry
	if database == nil {
		msg := "no valid database to read UID from"
		log.Error(msg)
		return 0, errors.New(msg)
	}
	username := viper.GetString(serviceConfigPath + ".account")
	uid, err := database.GetUIDByUsername(ctx, username)
	if err != nil {
		log.Error("Could not get UID by username")
		return 0, err
	}
	log.WithFields(log.Fields{
		"username": username,
		"uid":      uid,
	}).Debug("Got UID")
	return uint32(uid), nil
}

type workerRetryConfig struct {
	numRetries uint
	retrySleep time.Duration
}

func setAllWorkerRetryValues(workerRetryMap map[worker.WorkerType]workerRetryConfig) worker.ConfigOption {
	return func(c *worker.Config) error {
		for wt, wr := range workerRetryMap {
			// TODO When we upgrade to Go 1.24, maybe replace the below with an iterator that dynamically generates the
			// retry-specific worker-specific config options that are supported
			worker.SetWorkerSpecificConfigOption(wt, worker.NumRetriesOption, wr.numRetries)(c)
			worker.SetWorkerSpecificConfigOption(wt, worker.RetrySleepOption, wr.retrySleep)(c)
		}
		return nil
	}
}
