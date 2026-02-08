truncate images,deployments,vms,organisations cascade ;

insert into images (id, registry, repository, tag)
values ('00000000-0000-0000-0000-000000000000','docker.io', 'traefik/whoami', 'latest');

insert into vms (id, vcpus, memory, status, image_id, port, ip_address)
values ('00000000-0000-0000-0000-000000000000', 1, 1024, 'pending', '00000000-0000-0000-0000-000000000000', 80, '10.0.0.1/31');

-- ""SeEdS angeblich sollte das in ts ding sein oder so macht tom noch TODO