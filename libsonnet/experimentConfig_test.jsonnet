local test = import "jsonnetunit/test.libsonnet";
local exptConfig = import "experimentConfig.libsonnet";

test.suite({
    testSimpleCase: {
        actual: exptConfig.makeConfig(
            emails=["test1@example.com", "test2@example.com"],
            roleConfigs={
                "role1": exptConfig.makeRoleConfig(
                    account="testaccount",
                    destinationNodes=["testnode1", "testnode2"],
                )
            }
        ),
        expect: {
            "emails":  ["test1@example.com", "test2@example.com"],
            "roles": {
                "role1": {
                    "account": "testaccount",
                    "destinationNodes": ["testnode1", "testnode2"],
                },
            },
        },
    },
    testMultipleRolesCase: {
        actual: exptConfig.makeConfig(
            emails=["test1@example.com", "test2@example.com"],
            roleConfigs={
                "role1": exptConfig.makeRoleConfig(
                    account="testaccount1",
                    destinationNodes=["testnode1", "testnode2"],
                ),
                "role2": exptConfig.makeRoleConfig(
                    account="testaccount2",
                    destinationNodes=["testnode3", "testnode4"],
                ),
            },
        ),
        expect: {
            "emails": ["test1@example.com", "test2@example.com"],
            "roles": {
                "role1": {
                    "account": "testaccount1",
                    "destinationNodes": ["testnode1", "testnode2"],
                },
                "role2": {
                    "account": "testaccount2",
                    "destinationNodes": ["testnode3", "testnode4"],
                },
            },
        },
    },
    testOverridesCase: {
        actual: exptConfig.makeConfig(
            emails=["test1@example.com", "test2@example.com"],
            experimentOverride="realexperiment",
            roleConfigs={
                "testrole": exptConfig.makeRoleConfig(
                    account="testaccount",
                    destinationNodes=["testnode1", "testnode2"],
                    overrides={
                        keytabPathOverride: "/special/path/to/keytab",
                        userPrincipalOverride: "otheraccount/kerberos/principal@REALM",
                        desiredUIDOverride: 12345,
                        condorCreddHostOverride: "specialcreddhost.domain",
                        condorCollectorHostOverride: "specialcollectorhost.domain",
                        defaultRoleFileDestinationTemplateOverride: "/tmp/{{.DesiredUID}}_{{.Account}}",  # Any field in the worker.Config object is supported here
                        disableNotificationsOverride: false, # If true, no notifications will be sent for this role
                    },
                ),
            },
        ),
        expect: {
            "emails": ["test1@example.com", "test2@example.com"],
            "experimentOverride": "realexperiment",
            "roles": {
                "testrole": {
                    "account": "testaccount",
                    "destinationNodes": ["testnode1", "testnode2"],
                    keytabPathOverride: "/special/path/to/keytab",
                    userPrincipalOverride: "otheraccount/kerberos/principal@REALM",
                    desiredUIDOverride: 12345,
                    condorCreddHostOverride: "specialcreddhost.domain",
                    condorCollectorHostOverride: "specialcollectorhost.domain",
                    defaultRoleFileDestinationTemplateOverride: "/tmp/{{.DesiredUID}}_{{.Account}}",  # Any field in the worker.Config object is supported here
                    disableNotificationsOverride: false, # If true, no notifications will be sent for this role
                },
            },
        },
    },
    testInvalidOverrideCase: {
        actual: exptConfig.makeConfig(
            emails=["test1@example.com", "test2@example.com"],
            roleConfigs={
                "testrole": exptConfig.makeRoleConfig(
                    account="testaccount",
                    destinationNodes=["testnode1", "testnode2"],
                    overrides={
                        invalidOverride: "this should be ignored",
                    },
                ),
            },
        ),
        expect: {
            "emails": ["test1@example.com", "test2@example.com"],
            "roles": {
                "testrole": {
                    "account": "testaccount",
                    "destinationNodes": ["testnode1", "testnode2"],
                },
            },
        },
    },
    testExptAndRoleOverridesCase: {
        actual: exptConfig.makeConfig(
            emails=["test1@example.com", "test2@example.com"],
            experimentOverride="realexperiment",
            otherOverrides={
                disableNotificationsOverride: true, # If true, no notifications will be sent for this role
            },
            roleConfigs={
                "testrole": exptConfig.makeRoleConfig(
                    account="testaccount1",
                    destinationNodes=["testnode1", "testnode2"],
                    overrides={
                        disableNotificationsOverride: false, # If true, no notifications will be sent for this role
                    },
                ),
                "testrole2": exptConfig.makeRoleConfig(
                    account="testaccount2",
                    destinationNodes=["testnode3", "testnode4"],
                ),
            },
        ),
        expect: {
            "emails": ["test1@example.com", "test2@example.com"],
            "experimentOverride": "realexperiment",
            "roles": {
                "testrole": {
                    "account": "testaccount1",
                    "destinationNodes": ["testnode1", "testnode2"],
                    "disableNotificationsOverride": false, # If true, no notifications will be sent for this role
                },
                "testrole2": {
                    "account": "testaccount2",
                    "destinationNodes": ["testnode3", "testnode4"],
                    "disableNotificationsOverride": true, # If true, no notifications will be sent for this role
                },
            },
        },
    },
})
