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
	"context"
	"fmt"
	"strings"

	"github.com/fermitools/managed-tokens/internal/tracing"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	gomail "gopkg.in/gomail.v2"
)

// Email is an email message configuration
type email struct {
	from     string
	to       []string
	subject  string
	smtpHost string
	smtpPort int
}

func (e *email) From() string    { return e.from }
func (e *email) To() []string    { return e.to }
func (e *email) Subject() string { return e.subject }

// NewEmail returns an *email that can be used to send a message using SendMessage()
func NewEmail(from string, to []string, subject, smtpHost string, smtpPort int) *email {
	return &email{
		from:     from,
		to:       to,
		subject:  subject,
		smtpHost: smtpHost,
		smtpPort: smtpPort,
	}
}

// sendMessage sends message as an email based on the email object configuration
func (e *email) sendMessage(ctx context.Context, message string) error {
	ctx, span := otel.GetTracerProvider().Tracer("managed-tokens").Start(ctx, "notifications.email.sendMessage")
	span.SetAttributes(
		attribute.String("from", e.from),
		attribute.StringSlice("to", e.to),
		attribute.String("subject", e.subject),
		attribute.String("smtpHost", e.smtpHost),
		attribute.Int("smtpPort", e.smtpPort),
	)
	defer span.End()

	emailDialer := gomail.Dialer{
		Host: e.smtpHost,
		Port: e.smtpPort,
	}
	funcLogger := log.WithField("recipient", strings.Join(e.to, ", "))

	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", e.to...)
	m.SetHeader("Subject", e.subject)
	m.SetBody("text/plain", message)

	c := make(chan error)
	go func() {
		defer close(c)
		err := emailDialer.DialAndSend(m)
		c <- err
	}()

	select {
	case err := <-c:
		if err != nil {
			span.SetStatus(codes.Error, "Error sending email")
			funcLogger.WithField("email", e).Errorf("Error sending email: %s", err)
		} else {
			span.SetStatus(codes.Ok, "Sent email")
			funcLogger.Debug("Sent email")
		}
		return err
	case <-ctx.Done():
		err := ctx.Err()
		if err == context.DeadlineExceeded {
			tracing.LogErrorWithTrace(span, funcLogger, "Error sending email: timeout")
		} else {
			tracing.LogErrorWithTrace(span, funcLogger, fmt.Sprintf("Error sending email: %s", err.Error()))
		}
		return err
	}
}
