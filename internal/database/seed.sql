insert into images (id, registry, repository, tag, digest)
values ('00000000-0000-0000-0000-000000000000','docker.io', 'traefik/whoami', 'latest', 'yeet');

insert into vms (id, vcpus, memory, status, image_id, port, ip_address)
values ('00000000-0000-0000-0000-000000000000', 1, 1, 'pending', '00000000-0000-0000-0000-000000000000', 80, '10.0.0.1/31');

-- ""SeEdS angeblich sollte das in ts ding sein oder so macht tom noch TODO

CREATE EXTENSION IF NOT EXISTS btree_gist;
ALTER TABLE vms
    ADD CONSTRAINT exclude_overlapping_networks
        EXCLUDE USING gist (ip_address inet_ops WITH &&);

create or replace function zeitwork_notify() returns trigger as $$
    begin
        perform pg_notify(tg_table_name, NEW.id::text);
        RETURN NEW;
    end;
$$ language plpgsql;

create or replace trigger zeitwork_notify
    after insert or update on vms
    for each row execute function zeitwork_notify();

