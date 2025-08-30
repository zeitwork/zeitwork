connects to replication wal postgres log

publishes relevant events to nats

what is a relevant event:

1. a new build
2. route changes
3. vm instance changes
4. node changes (ie drain)

events:

- upsert build
- upsert domain
- upsert deployment
- upsert deployment_instance
- upsert instance
