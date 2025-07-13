import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";
import * as random from "@pulumi/random";

export class Postgres extends pulumi.ComponentResource {
    public readonly password: pulumi.Output<string>;
    public readonly secret: k8s.core.v1.Secret;
    public readonly pvc: k8s.core.v1.PersistentVolumeClaim;
    public readonly deployment: k8s.apps.v1.Deployment;
    public readonly service: k8s.core.v1.Service;

    constructor(name: string, args: { provider: k8s.Provider }, opts?: pulumi.ComponentResourceOptions) {
        super("zeitwork:component:Postgres", name, {}, opts);

        // Generate a random password
        const pw = new random.RandomPassword(name + "-pw", {
            length: 16,
            special: true,
        });
        this.password = pw.result;

        // Secret for postgres
        this.secret = new k8s.core.v1.Secret(name + "-secret", {
            metadata: { namespace: "zeitwork-system", name: name + "-secret" },
            stringData: {
                POSTGRES_PASSWORD: this.password,
            },
        }, { provider: args.provider, parent: this });

        // PVC for postgres data
        this.pvc = new k8s.core.v1.PersistentVolumeClaim(name + "-pvc", {
            metadata: { namespace: "zeitwork-system" },
            spec: {
                accessModes: ["ReadWriteOnce"],
                resources: { requests: { storage: "10Gi" } },
                storageClassName: "local-path"
            }
        }, { provider: args.provider, parent: this });

        // Deployment
        this.deployment = new k8s.apps.v1.Deployment(name + "-deployment", {
            metadata: { namespace: "zeitwork-system" },
            spec: {
                replicas: 1,
                selector: { matchLabels: { app: "postgres" } },
                template: {
                    metadata: { labels: { app: "postgres" } },
                    spec: {
                        containers: [{
                            name: "postgres",
                            image: "postgres:16",
                            ports: [{ containerPort: 5432 }],
                            env: [
                                { name: "POSTGRES_PASSWORD", valueFrom: { secretKeyRef: { name: name + "-secret", key: "POSTGRES_PASSWORD" } } }
                            ],
                            volumeMounts: [{
                                name: "pgdata",
                                mountPath: "/var/lib/postgresql/data"
                            }]
                        }],
                        volumes: [{
                            name: "pgdata",
                            persistentVolumeClaim: {
                                claimName: this.pvc.metadata.name,
                            }
                        }]
                    }
                }
            }
        }, { provider: args.provider, parent: this });

        // Service
        this.service = new k8s.core.v1.Service(name + "-svc", {
            metadata: { namespace: "zeitwork-system", name: name + "-svc" },
            spec: {
                selector: { app: "postgres" },
                ports: [{ port: 5432, targetPort: 5432 }],
                type: "ClusterIP"
            }
        }, { provider: args.provider, parent: this });

        this.registerOutputs();
    }
}

