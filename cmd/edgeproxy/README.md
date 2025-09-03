a plain go http proxy implementation that uses routes pull from the database at initial startup

connects to nats to listen to relevant changes

routes incoming traffic by host to a random healthy instance
