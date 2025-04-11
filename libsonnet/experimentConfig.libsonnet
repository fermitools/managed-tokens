// Object for experiment configurations, along with a function that creates this object

local supportedOverrides = [
    "experimentOverride",
    "keytabPathOverride",
    "userPrincipalOverride",
    "desiredUIDOverride",
    "condorCreddHostOverride",
    "condorCollectorHostOverride",
    "defaultRoleFileDestinationTemplateOverride",
    "disableNotificationsOverride",
];

{
    // makeConfig creates an experiment configuration object, and checks any overrides given by the caller
    // note: overrides is a dictionary of key-value pairs that will be merged into the experiment config
    makeConfig(emails, role, account, nodes, overrides={}): {
        emails: emails,
    } + {
            [if std.objectHas(overrides, "experimentOverride") then "experimentOverride"]:  overrides.experimentOverride ,
    } + {
        roles: {
            [role]: {
                account: account,
                destinationNodes: nodes,
            } + {
                [ov]: overrides[ov],
                for ov in std.objectFields(overrides)
                if ov != "experimentOverride" && std.member(supportedOverrides, ov)
            }
        },
    }
}
