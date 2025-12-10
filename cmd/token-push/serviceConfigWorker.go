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
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/fermitools/managed-tokens/internal/contextStore"
	"github.com/fermitools/managed-tokens/internal/worker"
)

// chansForWorkers is an interface that defines methods for obtaining channels
// used by worker.Workers. It includes methods to get channels for service
// configurations and success reporting.
type chansForWorkers interface {
	GetServiceConfigChan() chan<- *worker.Config
	GetSuccessChan() <-chan worker.SuccessReporter
}

// startServiceConfigWorkerForProcessing starts up the corresponding worker for the provided worker.WorkerType, gives it a set of channels to
// receive *worker.Configs and send notification.Notifications on, and sends *worker.Configs to the worker
func startServiceConfigWorkerForProcessing(ctx context.Context, wt worker.WorkerType,
	serviceConfigs map[string]*worker.Config, timeoutCheckKey timeoutKey) chansForWorkers {
	ctx, span := otel.GetTracerProvider().Tracer("token-push").Start(ctx, "startServiceConfigWorkerForProcessing")
	span.SetAttributes(
		attribute.KeyValue{
			Key:   "WorkerType",
			Value: attribute.StringValue(wt.String()),
		},
	)
	defer span.End()

	// If we don't have any serviceConfigs to process, don't start the worker
	if len(serviceConfigs) == 0 || serviceConfigs == nil {
		exeLogger.Debug("No serviceConfigs to process, not starting worker")
		return nil
	}

	// Make sure we're trying to start a valid worker
	if !slices.Contains(validWorkerTypes, wt) {
		exeLogger.WithField("workerType", wt.String()).Error("invalid worker type")
		return nil
	}
	w := wt.Worker()
	if w == nil {
		exeLogger.WithField("workerType", wt.String()).Error("no worker found for worker type")
		return nil
	}

	// We have a valid worker type, and serviceConfigs to process, so start everything up
	channels := worker.NewChannelsForWorkers(len(serviceConfigs))
	startListenerOnWorkerNotificationChans(ctx, channels.GetNotificationsChan())

	var useCtx context.Context
	if timeout, ok := timeouts[timeoutCheckKey]; ok {
		useCtx = contextStore.WithOverrideTimeout(ctx, timeout)
	} else {
		useCtx = ctx
	}

	// Start the work!
	go w(useCtx, channels)

	// Send our serviceConfigs to the worker
	for _, sc := range serviceConfigs {
		channels.GetServiceConfigChan() <- sc
	}
	close(channels.GetServiceConfigChan())

	return channels
}

// removeFailedServiceConfigs reads the worker.SuccessReporter chan from the passed in worker.ChannelsForWorkers object, and
// removes any *worker.Config objects from the passed in serviceConfigs map.  It returns a slice of the *worker.Configs that
// were removed
func removeFailedServiceConfigs(chans chansForWorkers, serviceConfigs map[string]*worker.Config) []*worker.Config {
	failedConfigs := make([]*worker.Config, 0, len(serviceConfigs))
	if chans == nil {
		exeLogger.Debug("No chans provided, nothing to remove from serviceConfigs")
		return failedConfigs
	}

	for workerSuccess := range chans.GetSuccessChan() {
		if !workerSuccess.GetSuccess() {
			exeLogger.WithField(
				"service", getServiceName(workerSuccess.GetService()),
			).Debug("Removing serviceConfig from list of configs to use")
			failedConfigs = append(failedConfigs, serviceConfigs[getServiceName(workerSuccess.GetService())])
			delete(serviceConfigs, getServiceName(workerSuccess.GetService()))
		}
	}
	return failedConfigs
}
