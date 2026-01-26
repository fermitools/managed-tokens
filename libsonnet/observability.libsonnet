// Observability settings (logs, metrics, tracing)
{
    local makeLogConfig(executable) = {
        logfile: "/var/log/"+executable+".log",
        debugfile: "/var/log/"+executable+".debug.log",
    },
    logs:  {
        "refresh-uids-from-ferry": makeLogConfig("refresh-uids-from-ferry"),
        "token-push": makeLogConfig("token-push"),
    },

    # Prometheus settings
    prometheus: {
        host: "hostname.domain",
        jobname: "managed_tokens",
    },

    # Loki settings
    loki: {
        host: "hostname.domain", # Required for loki use
        response_header_timeout: "1s", # Timeout for HTTP responses from Loki for each request
    },

    # Tracing settings
    tracing: {
        url: "scheme://hostname.domain", # Required for tracing use
    }
}
