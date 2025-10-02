NODEAGENT

a node agent starts with a NODE_ID, NODE_REGION_ID and NODE_DATABASE_URL. the node agent pulls the desired state and reconciles the towards the desired state. we run container using kata container. we have two runtime modes (later possible more), one is docker the other cloud hypervisor. we use kata container for the container management.

on first boot of the service we pull the node config from the database. and then start the reconciler loop

we pull the desired state from the database every 60s + a random offset between plusminus 15 seconds.

