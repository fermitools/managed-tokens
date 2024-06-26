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

package notifications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/fermitools/managed-tokens/internal/tracing"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// slackMessage is a Slack message configuration that consists of the url endpoint to which the message data should be POSTed via HTTP.
// Using a slackMessage assumes that a slack webhook or other HTTP POST API has been enabled on an existing slack channel
type slackMessage struct {
	url string
}

func (s *slackMessage) From() string   { return "" }
func (s *slackMessage) To() []string   { return []string{s.url} }
func (s *slackMessage) SetFrom() error { return nil }
func (s *slackMessage) SetTo(recipient []string) error {
	if len(recipient) > 1 {
		return errors.New("slackMessage does not support more than one recipient URL")
	}
	s.url = recipient[0]
	return nil
}

// NewSlackMessage returns a configured *slackMessage that can be used to send a message using SendMessage()
func NewSlackMessage(url string) *slackMessage {
	return &slackMessage{
		url: url,
	}
}

// sendMessage sends message as a Slack message by sending an HTTP POST request to the value of the url field of the
// slackMessage.
func (s *slackMessage) sendMessage(ctx context.Context, message string) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "notifications.slackMessage.sendMessage")
	defer span.End()

	if e := ctx.Err(); e != nil {
		tracing.LogErrorWithTrace(span, log.NewEntry(log.StandardLogger()), fmt.Sprintf("Error sending slack message: %s", e.Error()))
		return e
	}

	if message == "" {
		log.Warn("Slack message is empty.  Will not attempt to send it")
		return nil
	}

	msg := []byte(fmt.Sprintf(`{"text": "%s"}`, strings.Replace(message, "\"", "\\\"", -1)))
	req, err := http.NewRequest("POST", s.url, bytes.NewBuffer(msg))
	if err != nil {
		tracing.LogErrorWithTrace(span, log.NewEntry(log.StandardLogger()), fmt.Sprintf("Error sending slack message: %s", err.Error()))
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		tracing.LogErrorWithTrace(span, log.NewEntry(log.StandardLogger()), fmt.Sprintf("Error sending slack message: %s", err.Error()))
		return err
	}

	// This should be redundant, but just in case the timeout before didn't trigger.
	if e := ctx.Err(); e != nil {
		tracing.LogErrorWithTrace(span, log.NewEntry(log.StandardLogger()), fmt.Sprintf("Error sending slack message: %s", e.Error()))
		return e
	}

	defer resp.Body.Close()

	// Parse the response to make sure we're good
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := errors.New("could not send slack message")
		log.WithFields(log.Fields{
			"url":              s.url,
			"response status":  resp.Status,
			"response headers": resp.Header,
			"response body":    string(body),
		}).Error(err)
		span.SetAttributes(
			attribute.String("url", s.url),
			attribute.String("response status", resp.Status),
		)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	log.Debug("Slack message sent")
	span.SetStatus(codes.Ok, "Slack message sent")
	return nil
}
