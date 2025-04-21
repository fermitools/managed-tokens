// This test is meant to be used to check the entirety of the managedTokens.jsonnet file.
// Right now, the "actual" section should just be a copy-paste of the managedTokens.jsonnet file after the imports, and
// the expected should be the expected output from running jsonnet on the managedTokens.jsonnet file.

// Run by using the command: jsonnet -J libsonnet/jsonnetunit managedTokens_test.jsonnet

// Currently, this process is a bit manual, but at least it gives us a test


local test = import 'jsonnetunit/test.libsonnet';

local exptConfig = import 'libsonnet/experimentConfig.libsonnet';
local ferryConfig = import 'libsonnet/ferryConfig.libsonnet';
local timeoutsConfig = import 'libsonnet/timeoutsConfig.libsonnet';
local emailConfig = import 'libsonnet/emailConfig.libsonnet';
local obsConfig = import 'libsonnet/observability.libsonnet';
local notificationsConfig = import 'libsonnet/notificationsConfig.libsonnet';


test.suite({
    testSampleConfig: {
        // This should just be a copy of the managedTokens.jsonnet contents after the imports
        actual: {
            # Experiment config items
            experiments: {
                // Minimum-required configs
                dune: exptConfig.makeConfig(
                    emails=["email1@example.com"],
                    roleConfigs={
                        "production": exptConfig.makeRoleConfig(
                            account="dunepro",
                            destinationNodes=["dune01", "dune02", "dune03"],
                        ),
                    }
                ),
                mu2e: exptConfig.makeConfig(
                    emails=["email2@example.com"],
                    roleConfigs={
                        "production": exptConfig.makeRoleConfig(
                            account="mu2epro",
                            destinationNodes=["mu2e01", "mu2e02", "mu2e03"],
                        ),
                    }
                ),
                // Configs with multiple roles
                "sbnd": exptConfig.makeConfig(
                    emails=["email1@example.com"],
                    roleConfigs={
                        "production": exptConfig.makeRoleConfig(
                            account="sbndpro",
                            destinationNodes=["sbnd01", "sbnd02", "sbnd03"],
                        ),
                        "testrole": exptConfig.makeRoleConfig(
                            account="sbndtestrole",
                            destinationNodes=["sbnd01", "sbnd02", "sbnd03"],
                        ),
                    }
                ),
                // Configs with overrides
                "dune-2": exptConfig.makeConfig(
                    emails=["email1@example.com"],
                    experimentOverride="dune",
                    roleConfigs={
                        "production": exptConfig.makeRoleConfig(
                            account="dunepro",
                            destinationNodes=["dune01", "dune02", "dune03"],
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
                ),
                // Config with experiment-level overrides and role-level overrides:
                "sbnd-2": exptConfig.makeConfig(
                    emails=["email2@example.com"],
                    experimentOverride="sbnd",
                    otherOverrides = {
                        disableNotificationsOverride: true, # If true, no notifications will be sent for this role
                    },
                    roleConfigs={
                        "production": exptConfig.makeRoleConfig(
                            account="sbndpro",
                            destinationNodes=["sbnd01", "sbnd02", "sbnd03"],
                            overrides={
                                keytabPathOverride: "/special/path/to/keytab",
                                userPrincipalOverride: "sbndpro/kerberos/principal@REALM",
                                desiredUIDOverride: 12345,
                                condorCreddHostOverride: "specialcreddhost.domain",
                                condorCollectorHostOverride: "specialcollectorhost.domain",
                                defaultRoleFileDestinationTemplateOverride: "/tmp/{{.DesiredUID}}_{{.Account}}",  # Any field in the worker.Config object is supported here
                                disableNotificationsOverride: false, # If true, no notifications will be sent for this role
                            },
                        ),
                        "testrole": exptConfig.makeRoleConfig(
                            account="sbndtest",
                            destinationNodes=["sbnd01", "sbnd02", "sbnd03"],
                        ),
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
        },
        // With any edits to the sample jsonnet file, we should update the expected results as well
        expect: {
            "condorCollectorHost": "collectorhost.domain",
            "condorScheddConstraint": "my_constraint",
            "dbLocation": "/var/lib/managed-tokens/managed-tokens.db",
            "defaultRoleFileDestinationTemplate": "/tmp/{{.DesiredUID}}_{{.Account}}",
            "devEnvironmentLabel": "development",
            "disableNotifications": false,
            "email": {
                "from": "admin_email@example.com",
                "smtpHost": "localhost",
                "smtpPort": 25
            },
            "errorCountToSendMessage": 3,
            "experiments": {
                "dune": {
                    "emails": [
                        "email1@example.com"
                    ],
                    "roles": {
                        "production": {
                        "account": "dunepro",
                        "destinationNodes": [
                            "dune01",
                            "dune02",
                            "dune03"
                        ]
                        }
                    }
                },
                "dune-2": {
                    "emails": [
                        "email1@example.com"
                    ],
                    "experimentOverride": "dune",
                    "roles": {
                        "production": {
                        "account": "dunepro",
                        "condorCollectorHostOverride": "specialcollectorhost.domain",
                        "condorCreddHostOverride": "specialcreddhost.domain",
                        "defaultRoleFileDestinationTemplateOverride": "/tmp/{{.DesiredUID}}_{{.Account}}",
                        "desiredUIDOverride": 12345,
                        "destinationNodes": [
                            "dune01",
                            "dune02",
                            "dune03"
                        ],
                        "disableNotificationsOverride": false,
                        "keytabPathOverride": "/special/path/to/keytab",
                        "userPrincipalOverride": "dunepro/kerberos/principal@REALM"
                        }
                    }
                },
                "mu2e": {
                    "emails": [
                        "email2@example.com"
                    ],
                    "roles": {
                        "production": {
                        "account": "mu2epro",
                        "destinationNodes": [
                            "mu2e01",
                            "mu2e02",
                            "mu2e03"
                        ]
                        }
                    }
                },
                "sbnd": {
                    "emails": [
                        "email1@example.com"
                    ],
                    "roles": {
                        "production": {
                        "account": "sbndpro",
                        "destinationNodes": [
                            "sbnd01",
                            "sbnd02",
                            "sbnd03"
                        ]
                        },
                        "testrole": {
                        "account": "sbndtestrole",
                        "destinationNodes": [
                            "sbnd01",
                            "sbnd02",
                            "sbnd03"
                        ]
                        }
                    }
                },
                "sbnd-2": {
                    "emails": [
                        "email2@example.com"
                    ],
                    "experimentOverride": "sbnd",
                    "roles": {
                        "production": {
                        "account": "sbndpro",
                        "condorCollectorHostOverride": "specialcollectorhost.domain",
                        "condorCreddHostOverride": "specialcreddhost.domain",
                        "defaultRoleFileDestinationTemplateOverride": "/tmp/{{.DesiredUID}}_{{.Account}}",
                        "desiredUIDOverride": 12345,
                        "destinationNodes": [
                            "sbnd01",
                            "sbnd02",
                            "sbnd03"
                        ],
                        "disableNotificationsOverride": false,
                        "keytabPathOverride": "/special/path/to/keytab",
                        "userPrincipalOverride": "sbndpro/kerberos/principal@REALM"
                        },
                        "testrole": {
                        "account": "sbndtest",
                        "destinationNodes": [
                            "sbnd01",
                            "sbnd02",
                            "sbnd03"
                        ],
                        "disableNotificationsOverride": true
                        }
                    }
                }
            },
            "ferry": {
                "caPath": "path/to/certificates",
                "host": "ferryhost.domain",
                "hostCert": "/path/to/hostcert",
                "hostKey": "/path/to/hostkey",
                "port": 8445,
                "serviceExperiment": "fermilab",
                "serviceKerberosPrincipal": "/path/to/service_principal",
                "serviceKeytabPath": "/path/to/service_kerberos_keytab",
                "serviceRole": "",
                "vaultServer": "vaultserver.domain"
            },
            "fileCopierOptions": "--perms --chmod=u=r,go=",
            "kerberosPrincipalPattern": "principal_pattern",
            "keytabPath": "/opt/managed-tokens/keytabs",
            "logs": {
                "refresh-uids-from-ferry": {
                    "debugfile": "/var/log/refresh-uids-from-ferry.debug.log",
                    "logfile": "/var/log/refresh-uids-from-ferry.log"
                },
                "token-push": {
                    "debugfile": "/var/log/token-push.debug.log",
                    "logfile": "/var/log/token-push.log"
                }
            },
            "loki": {
                "host": "hostname.domain"
            },
            "minTokenLifetime": "3d",
            "notifications": {
                "SLACK_ALERTS_URL": "https://hooks.slack.com/FILL_IN_URL_HERE",
                "admin_email": "admin@example.com"
            },
            "notifications_test": {
                "SLACK_ALERTS_URL": "https://hooks.slack.com/FILL_IN_URL_HERE",
                "admin_email": "admin@example.com"
            },
            "pingOptions": "--arg1 --arg2 value2",
            "prometheus": {
                "host": "hostname.domain",
                "jobname": "managed_tokens"
            },
            "serviceCreddVaultTokenPathRoot": "/var/lib/managed-tokens/service-credd-vault-tokens",
            "sshOptions": "-o Arg1=val1 -o Arg2=val2",
            "timeouts": {
                "ferryRequestTimeout": "30s",
                "globalTimeout": "300s",
                "kerberosTimeout": "20s",
                "pingTimeout": "10s",
                "pushTimeout": "30s",
                "vaultStorerTimeout": "60s"
            },
            "tracing": {
                "url": "scheme://hostname.domain"
            },
            "workerType": {
                "getKerberosTickets": {
                    "numRetries": 0,
                    "retrySleep": "0s"
                },
                "pingAggregator": {
                    "numRetries": 0,
                    "retrySleep": "0s"
                },
                "pushTokens": {
                    "numRetries": 3,
                    "retrySleep": "60s"
                },
                "storeAndGetToken": {
                    "numRetries": 0,
                    "retrySleep": "0s"
                },
                "storeAndGetTokenInteractive": {
                    "numRetries": 0,
                    "retrySleep": "0s"
                }
            }
        }
    },
})
