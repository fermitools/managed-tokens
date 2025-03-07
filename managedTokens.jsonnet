local exptConfig = import 'experimentConfig.libsonnet';

{
    # Experiment config items
    experiments: {
        // Minimum-required configs
        dune: exptConfig.makeConfig(
            emails=["email1@example.com"],
            role="production",
            account="dunepro",
            nodes=["dune01.fnal.gov", "dune02", "dune03"],
            overrides={},
        ),
        mu2e: exptConfig.makeConfig(
            emails=["email2@example.com"],
            role="production",
            account="mu2epro",
            nodes=["mu2e01.fnal.gov", "mu2e02", "mu2e03"],
            overrides={},
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
    timeouts: {
        globalTimeout: "300s",
        kerberosTimeout: "20s",
        vaultStorerTimeout: "60s",
        pingTimeout: "10s",
        pushTimeout: "30s",
        ferryRequestTimeout: "30s",
    },

    minTokenLifetime: "3d", # If our vault token has less than this time left, get a new one


    # Worker-specific configurations
    workerType: {
        pushTokens: {
            numRetries: 3,
            retrySleep: "60s",
        }
    } + {
        // All these use default configuration values
        // of 0 retries
        [x]: {
            numRetries: 0,
            retrySleep: "0s",
        }
        for x in [
            "getKerberosTickets",
            "storeAndGetToken",
            "storeAndGetTokenInteractive",
            "pingAggregator",
        ]
    },


    # FERRY config
    ferry: {
        host: "ferryhost.domain",
        port: 8445,
        caPath: "path/to/certificates",
        hostCert: "/path/to/hostcert",
        hostKey: "/path/to/hostkey",
        serviceExperiment: "fermilab",
        serviceRole:"",
        serviceKerberosPrincipal: "/path/to/service_principal",
        serviceKeytabPath: "/path/to/service_kerberos_keytab",
        vaultServer: "vaultserver.domain",
    },

    # Email settings
    email: {
        from: "admin_email@example.com",
        smtpHost: "localhost",
        smtpPort: 25,
    },

    # Logfiles
    logs: {
        [x]: {
            logfile: "/var/log/"+x+".log",
            debugfile: "/var/log/"+x+".debug.log",
        }
        for x in [
            "refresh-uids-from-ferry",
            "token-push",
        ]
    },

    # Prometheus settings
    prometheus: {
        host: "hostname.domain",
        jobname: "managed_tokens",
    },

    # Loki settings
    loki: {
        host: "hostname.domain", # Required for loki use
    },

    # Tracing settings
    tracing: {
        url: "scheme://hostname.domain", # Required for tracing use
    },

    # Notifications
    notifications: {
        SLACK_ALERTS_URL: "https://hooks.slack.com/FILL_IN_URL_HERE",
        admin_email: "admin@example.com",
    },

    # Same as above, but used in test runs
    notifications_test: {
        SLACK_ALERTS_URL: "https://hooks.slack.com/FILL_IN_URL_HERE",
        admin_email: "admin@example.com",
    },


    # Global config items that won't change as much
    keytabPath: "/opt/managed-tokens/keytabs",
    condorCollectorHost: "collectorhost.domain",
    condorScheddConstraint: "my_constraint",
    vaultServer: "vaultserver.domain",
    serviceCreddVaultTokenPathRoot: "/var/lib/managed-tokens/service-credd-vault-tokens",
    kerberosPrincipalPattern: "principal_pattern",
    dbLocation: "/var/lib/managed-tokens/managed-tokens.db",
    errorCountToSendMessage: 3,
    defaultRoleFileDestinationTemplate: "/tmp/{{.DesiredUID}}_{{.Account}}", # Any field in the worker.Config object is supported here,
    pingOptions: "--arg1 --arg2 value2", # Options to use with ping,
    fileCopierOptions: "--perms --chmod=u=r,go=", # Extra options to give to the fileCopier utility - usually rsync,
    sshOptions: "-o Arg1=val1 -o Arg2=val2", # Options to use with fileCopier to establish the SSH connection
    disableNotifications: false, # If true, no notifications will be sent

    # Optional, and should not be used in production.  Defaults to "production", but can be specified here
    # or with environment variable MANAGED_TOKENS_DEV_ENVIRONMENT_LABEL
    devEnvironmentLabel: "development",
}
