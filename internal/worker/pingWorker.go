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
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/fermitools/managed-tokens/internal/metrics"
	"github.com/fermitools/managed-tokens/internal/notifications"
	"github.com/fermitools/managed-tokens/internal/ping"
	"github.com/fermitools/managed-tokens/internal/service"
	"github.com/fermitools/managed-tokens/internal/tracing"
	"github.com/fermitools/managed-tokens/internal/utils"
)

var (
	pingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "managed_tokens",
			Name:      "ping_duration_seconds",
			Help:      "Duration (in seconds) to ping a node",
		},
		[]string{
			"node",
		},
	)
	pingFailureCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "managed_tokens",
		Name:      "failed_ping_count",
		Help:      "The number of times the Managed Tokens Service failed to ping a node",
	},
		[]string{
			"node",
		},
	)
)

const pingDefaultTimeoutStr string = "10s"

func init() {
	metrics.MetricsRegistry.MustRegister(pingDuration)
	metrics.MetricsRegistry.MustRegister(pingFailureCount)
}

// pingSuccess is a type that conveys whether PingAggregatorWorker successfully pings all the configured destination nodes for each service
type pingSuccess struct {
	service.Service
	success bool
}

func (p *pingSuccess) GetService() service.Service {
	return p.Service
}

func (p *pingSuccess) GetSuccess() bool {
	return p.success
}

// PingAggregatorWorker is a worker that listens on chans.GetServiceConfigChan(), and for the received worker.Config objects,
// concurrently pings all of the Config's destination nodes.  It returns when chans.GetServiceConfigChan() is closed,
// and it will in turn close the other chans in the passed in ChannelsForWorkers
func PingAggregatorWorker(ctx context.Context, chans ChannelsForWorkers) {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.PingAggregatorWorker")
	defer span.End()

	defer close(chans.GetSuccessChan())
	defer func() {
		close(chans.GetNotificationsChan())
		log.Debug("Closed PingAggregatorWorker Notifications Chan")
	}()
	var wg sync.WaitGroup
	defer wg.Wait() // Don't close the NotificationsChan or SuccessChan until we're done sending notifications and success statuses

	pingTimeout, err := utils.GetProperTimeoutFromContext(ctx, pingDefaultTimeoutStr)
	if err != nil {
		log.Fatal("Could not parse ping timeout")
	}

	for sc := range chans.GetServiceConfigChan() {
		wg.Add(1)
		go func(sc *Config) {
			defer wg.Done()

			ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.PingAggregatorWorker_anonFunc")
			span.SetAttributes(
				attribute.String("service", sc.ServiceNameFromExperimentAndRole()),
			)
			defer span.End()

			success := &pingSuccess{
				Service: sc.Service,
			}
			serviceLogger := log.WithField("service", sc.Service.Name())

			defer func(p *pingSuccess) {
				chans.GetSuccessChan() <- p
			}(success)

			// Prepare slice of PingNoders
			nodes := make([]ping.PingNoder, 0, len(sc.Nodes))
			for _, node := range sc.Nodes {
				nodes = append(nodes, ping.NewNode(node))
			}

			var extraPingOpts []string
			extraPingOpts, ok := GetPingOptionsFromExtras(sc)
			if !ok {
				extraPingOpts = make([]string, 0)
			}

			pingContext, pingCancel := context.WithTimeout(ctx, pingTimeout)
			defer pingCancel()
			pingStatus := pingAllNodes(pingContext, extraPingOpts, nodes...)

			failedNodes := make([]ping.PingNoder, 0, len(sc.Nodes))
			for status := range pingStatus {
				nodeLogger := serviceLogger.WithField("node", status.PingNoder.String())
				if status.Err != nil {
					var msg string
					if errors.Is(status.Err, context.DeadlineExceeded) {
						msg = "Timeout error"
					} else {
						msg = "Error pinging node"
					}
					nodeLogger.Error(msg)
					failedNodes = append(failedNodes, status.PingNoder)
					sc.RegisterUnpingableNode(status.PingNoder.String())
				} else {
					nodeLogger.Debug("Successfully pinged node")
				}
			}

			if len(failedNodes) != 0 {
				failedNodesStrings := make([]string, 0, len(failedNodes))
				for _, node := range failedNodes {
					failedNodesStrings = append(failedNodesStrings, node.String())
				}
				msg := "Could not ping the following nodes: " + strings.Join(failedNodesStrings, ", ")
				tracing.LogErrorWithTrace(span, serviceLogger, msg)
				chans.GetNotificationsChan() <- notifications.NewSetupError(
					msg,
					sc.ServiceNameFromExperimentAndRole(),
				)
				return
			}
			success.success = true
			tracing.LogSuccessWithTrace(span, serviceLogger, "Successfully pinged all nodes for service")
		}(sc)
	}
}

// pingAllNodes will launch goroutines, which each ping a ping.PingNoder from the nodes variadic.  It returns a channel,
// on which it reports the ping.pingNodeStatuses signifying success or error
func pingAllNodes(ctx context.Context, extraPingOpts []string, nodes ...ping.PingNoder) <-chan ping.PingNodeStatus {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.pingAllNodes")
	defer span.End()

	// Buffered Channel to report on
	c := make(chan ping.PingNodeStatus, len(nodes))
	var wg sync.WaitGroup
	wg.Add(len(nodes))
	for _, n := range nodes {
		go func(n ping.PingNoder) {
			defer wg.Done()

			start := time.Now()
			ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "worker.pingAllNodes_anonFunc")
			span.SetAttributes(attribute.String("node", n.String()))
			defer span.End()

			p := ping.PingNodeStatus{
				PingNoder: n,
				Err:       n.PingNode(ctx, extraPingOpts),
			}
			if p.Err != nil {
				span.SetStatus(codes.Error, "Failed to ping node")
				pingFailureCount.WithLabelValues(n.String()).Inc()
			} else {
				span.SetStatus(codes.Ok, "Successfully pinged node")
				dur := time.Since(start).Seconds()
				pingDuration.WithLabelValues(n.String()).Observe(dur)
			}
			c <- p
		}(n)
	}

	// Wait for all goroutines to finish, then close channel so that the caller knows all objects have been sent
	go func() {
		defer close(c)
		wg.Wait()
	}()

	return c
}
