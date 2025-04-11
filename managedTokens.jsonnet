local exptConfig = import 'libsonnet/experimentConfig.libsonnet';
local ferryConfig = import 'libsonnet/ferryConfig.libsonnet';
local timeoutsConfig = import 'libsonnet/timeoutsConfig.libsonnet';
local emailConfig = import 'libsonnet/emailConfig.libsonnet';
local obsConfig = import 'libsonnet/observability.libsonnet';
local notificationsConfig = import 'libsonnet/notificationsConfig.libsonnet';

{
    # Experiment config items
    experiments: {
        // Minimum-required configs
        dune: exptConfig.makeConfig(
            emails=["email1@example.com"],
            role="production",
            account="dunepro",
            nodes=["dune01.fnal.gov", "dune02", "dune03"],
        ),
        mu2e: exptConfig.makeConfig(
            emails=["email2@example.com"],
            role="production",
            account="mu2epro",
            nodes=["mu2e01.fnal.gov", "mu2e02", "mu2e03"],
        ),
        // Configs with overrides
        "dune-2": exptConfig.makeConfig(
            emails=["email1@example.com"],
            role="production",
            account="dunepro",
            nodes=["dune01.fnal.gov", "dune02", "dune03"],
            overrides={
                experimentOverride: "dune",
                keytabPathOverride: "/special/path/to/keytab",
                userPrincipalOverride: "dunepro/kerberos/principal@REALM",
                desiredUIDOverride: 12345,
                condorCreddHostOverride: "specialcreddhost.domain",
                condorCollectorHostOverride: "specialcollectorhost.domain",
                defaultRoleFileDestinationTemplateOverride: "/tmp/{{.DesiredUID}}_{{.Account}}",  # Any field in the worker.Config object is supported here
                disableNotificationsOverride: false, # If true, no notifications will be sent for this role
            },
        ),
    },

    # Performance/timeouts
    timeouts: timeoutsConfig,
    minTokenLifetime: "3d", # If our vault token has less than this time left, get a new one


    # Worker-specific configurations
    # Make changes using makeWorkerTypeConfig function
    local makeWorkerTypeConfig(numRetries=0, retrySleep="0s") = {
       numRetries: numRetries,
       retrySleep: retrySleep,
    },
    workerType: {
        pushTokens: makeWorkerTypeConfig(numRetries=3, retrySleep="60s"),
        getKerberosTickets: makeWorkerTypeConfig(), // Uses default values
        storeAndGetToken: makeWorkerTypeConfig(),
        storeAndGetTokenInteractive: makeWorkerTypeConfig(),
        pingAggregator: makeWorkerTypeConfig(),
    },

    # Observability
    logs: obsConfig.logs,
    prometheus: obsConfig.prometheus,
    loki: obsConfig.loki,
    tracing: obsConfig.tracing,

    # Notifications
    email: emailConfig,
    notifications: notificationsConfig.prod,
    notifications_test: notificationsConfig.test,
    errorCountToSendMessage: 3,

    ferry: ferryConfig,

    # Global config items that won't change as much
    keytabPath: "/opt/managed-tokens/keytabs",
    condorCollectorHost: "collectorhost.domain",
    condorScheddConstraint: "my_constraint",
    serviceCreddVaultTokenPathRoot: "/var/lib/managed-tokens/service-credd-vault-tokens",
    kerberosPrincipalPattern: "principal_pattern",
    dbLocation: "/var/lib/managed-tokens/managed-tokens.db",
    defaultRoleFileDestinationTemplate: "/tmp/{{.DesiredUID}}_{{.Account}}", # Any field in the worker.Config object is supported here,
    pingOptions: "--arg1 --arg2 value2", # Options to use with ping,
    fileCopierOptions: "--perms --chmod=u=r,go=", # Extra options to give to the fileCopier utility - usually rsync,
    sshOptions: "-o Arg1=val1 -o Arg2=val2", # Options to use with fileCopier to establish the SSH connection
    disableNotifications: false, # If true, no notifications will be sent

    # Optional, and should not be used in production.  Defaults to "production", but can be specified here
    # or with environment variable MANAGED_TOKENS_DEV_ENVIRONMENT_LABEL
    devEnvironmentLabel: "development",
}
