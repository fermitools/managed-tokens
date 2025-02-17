[![Go Report Card](https://goreportcard.com/badge/github.com/fermitools/managed-tokens)](https://goreportcard.com/report/github.com/fermitools/managed-tokens)
![Go build and test](https://github.com/fermitools/managed-tokens/actions/workflows/go_test.yml/badge.svg)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/fermitools/managed-tokens)](https://pkg.go.dev/github.com/fermitools/managed-tokens)


# managed-tokens
Managed Tokens service for Condor/Hashicorp Vault Token Distribution

The Managed Tokens Service stores and distributes HashiCorp Vault tokens for stakeholders to use in their automated grid computing activities. Specifically, the Managed Tokens service enables stakeholders to automate grid activities such as job submission and file transfers by ensuring that the valid credentials (Vault Tokens) always exist on submit nodes, ready to be used.

# Executables
The Managed Tokens Service consists of two executables:

* `token-push`: Executable that uses the service keytabs to generate vault tokens, store them on [HTCondor](https://htcondor.org/) credd machines, and push the vault tokens to the experiment interactive nodes. By default, this runs every hour.
* `refresh-uids-from-ferry`: Executable that queries FERRY (the credentials and grid mapping registry service at Fermilab) to pull down the applicable UIDs for the configured UNIX accounts. By default, this runs daily each morning.

The first time the operator of a Managed Tokens service deployment runs `token-push` for a given service (experiment-role combination), the operator needs to pass the `-r/--run-onboarding` flag, with `-s <SERVICE>` also specified.  This will enable the operator to authenticate with the token issuer and generate the refresh token that will eventually be used to generate new bearer tokens and vault tokens.

The `token-push` executable will copy the vault token to the destination nodes at two locations:

* `/tmp/vt_u<UID>`
* `/tmp/vt_u<UID>-<service>`

# Notifications and Stakeholder-Specific Emails

The Managed Tokens Service, under the default mode, will send errors and pertinent warnings to three places:

* Stakeholder-specific alerts will go to the recipients specified in the emails entry for a stakeholder in the configuration file.
* All alerts will get sent to configured `admin_email` (logging level ERROR or above).
* All alerts will get sent to the configured slack channel

Notifications can be disabled globally or by stakeholder via the configuration file, or globally with the `--dont-notify`/`--disable-notifications` flag
passed to the command line.

# Monitoring

## Logs

The logfiles for the Managed Tokens service are, by default, located in the /var/log/managed-tokens directory (configurable). Each executable has its own log and debug log, and these are rotated periodically by default if installed via RPM.

## Metrics

These are the current prometheus metrics that can be pushed from the Managed Tokens executables to a [prometheus pushgateway](https://prometheus.io/docs/practices/pushing/) configured at the `prometheus.host` entry in the configuration file. These are:

### General executable-level metrics
* `managed_tokens_stage_duration_seconds`:  Per executable, per stage (setup, processing, cleanup).  How long each stage took to run.

### refresh-uids-from-ferry-specific metrics

* `managed_tokens_last_ferry_refresh`: Timestamp of when refresh-uids-from-ferry executable last got information from FERRY.
* `managed_tokens_ferry_request_duration_seconds`: The amount of time it took in seconds to make a request to FERRY and receive the response
* `managed_tokens_ferry_request_error_count`: The number of requests to FERRY that failed

### token-push-specific metrics

* `managed_tokens_failed_services_push_count`:  Count of how many services registered a failure to push a vault token to a node in the current run of token-push.  Basically, a failure count.

### Internal library metrics

#### Kerberos metrics
* `managed_tokens_kinit_duration_seconds`: Duration (in seconds) for a kerberos ticket to be created from the service principal
* `managed_tokens_failed_kinit_count`: The number of times the Managed Tokens Service failed to create a kerberos ticket from the service principal

#### Vault Token Store metrics
* `managed_tokens_last_token_store_timestamp`: Timestamp of the last successful store of a service vault token in a condor credd by the Managed Tokens Service
* `managed_tokens_token_store_duration_seconds`: Duration (in seconds) for a vault token to get stored in a condor credd
* `managed_tokens_failed_vault_token_store_count`: The number of times the Managed Tokens Service failed to store a vault token in a condor credd

#### Node-pinging metrics
* `managed_tokens_ping_duration_seconds`: Duration (in seconds) to ping a node
* `managed_tokens_failed_ping_count`: The number of times the Managed Tokens Service failed to ping a node

#### Pushing tokens metrics
* `managed_tokens_last_token_push_timestamp`: Timestamp of when token-push last pushed a particular service vault token to a particular node.
* `managed_tokens_token_push_duration_seconds`: Duration (in seconds) for a vault token to get pushed to a node
* `managed_tokens_failed_token_push_count`: The number of times the Managed Tokens service failed to push a token to an interactive node


#### Notification-sending metrics
* `managed_tokens_admin_error_email_last_sent_timestamp`:  The last time managed tokens service attempted to send an admin error notification
* `managed_tokens_admin_error_email_send_duration_seconds`: Time in seconds it took to successfully send an admin error email
* `managed_tokens_service_error_email_last_sent_timestamp`: Last time managed tokens service attempted to send an service error notification
* `managed_tokens_service_error_email_send_duration_seconds`: Time in seconds it took to successfully send a service error email


#### Error-count metrics (mirrors internal database state)
* `managed_tokens_current_setup_error_count`: Count of how many consecutive setup errors there have been for a single service.  Will reset to 0 after an error notification is sent
* `managed_tokens_current_push_error_count`: Count of how many consecutive push errors there have been for a single service/node combination.  Will reset to 0 after an error notification is sent

## Tracing
The Managed Tokens service, starting with version v0.14, includes OpenTelemetry tracing in both the executables and the libraries.  The provided OpenTelemetry-compatible trace provider uses a [Jaeger](https://www.jaegertracing.io/)  for trace collection.  In the future, the plan is to migrate this
trace provider to the standard OpenTelemetry trace provider.

The tracing endpoint can be configured in the configuration file, under the `tracing.url` entry.
