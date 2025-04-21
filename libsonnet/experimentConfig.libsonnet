// Object for experiment configurations, along with a function that creates this object

local supportedOverrides = [
    "keytabPathOverride",
    "userPrincipalOverride",
    "desiredUIDOverride",
    "condorCreddHostOverride",
    "condorCollectorHostOverride",
    "defaultRoleFileDestinationTemplateOverride",
    "disableNotificationsOverride",
];

{
    // func makeConfig(emails []string, roleConfigs map[string]roleConfig, experimentOverride string, otherOverrides map[string]any)
    // makeConfig creates an experiment configuration object, and checks any overrides given by the caller
    // note: overrides is a dictionary of key-value pairs that will be merged into the experiment config
    //
    // At minimum, the caller must provide a list of emails and a map of roleConfigs
    // It is recommended to use makeRoleConfig to create roleConfig objects
    //
    // If overrides are given both in the roleConfigs and in otherOverrides here,
    // the roleConfigs will take precedence
    makeConfig(emails, roleConfigs, experimentOverride="", otherOverrides={}): {
        emails: emails,
    } + {
            [if experimentOverride != "" then "experimentOverride"]: experimentOverride,
    } + {
        roles: {
            [r]: otherOverrides + roleConfigs[r] // otherOverrides goes first so that it can be overridden by the roleConfig's overrides field
            for r in std.objectFields(roleConfigs)
        },
    },
    // func makeRoleConfig(account string, nodes []string, overrides map[string]any)
    // makeRoleConfig creates a role configuration object, and checks any overrides
    // given by the caller against the supported overrides
    //
    // This is the recommended way to create roleConfig objects
    makeRoleConfig(account, destinationNodes, overrides={}): {
            account: account,
            destinationNodes: destinationNodes,
    } + {
            [ov]: overrides[ov],
            for ov in std.objectFields(overrides)
            if std.member(supportedOverrides, ov)
    }
}
