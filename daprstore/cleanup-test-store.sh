redis-cli --scan --pattern "dapr-limiter-test*" | xargs redis-cli del