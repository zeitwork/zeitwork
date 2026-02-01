-- Custom SQL migration file, put your code below! --
ALTER TABLE vms ADD CONSTRAINT exclude_overlapping_networks EXCLUDE USING gist (ip_address inet_ops WITH &&)
