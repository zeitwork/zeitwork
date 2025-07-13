import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";



export class Registry extends pulumi.ComponentResource {
    public readonly pvc: k8s.core.v1.PersistentVolumeClaim;
    public readonly deployment: k8s.apps.v1.Deployment;
    public readonly certificate: k8s.apiextensions.CustomResource;
    public readonly proxy: k8s.apiextensions.CustomResource;
    public readonly service: k8s.core.v1.Service;

    constructor(name: string, args: {}, opts?: pulumi.ComponentResourceOptions) {
        super("zeitwork:component:Registry", name, {}, opts);

        this.pvc = new k8s.core.v1.PersistentVolumeClaim(name + "-pvc", {
            metadata: { namespace: "zeitwork-system" },
            spec: {
                accessModes: ["ReadWriteOnce"],
                resources: { requests: { storage: '10G' } },
                storageClassName: "local-path"
            }
        }, { parent: this });

        this.deployment = new k8s.apps.v1.Deployment(name + "-deployment", {
            metadata: { namespace: "zeitwork-system" },
            spec: {
                replicas: 1,
                selector: { matchLabels: { app: "registry" } },
                template: {
                    metadata: { labels: { app: "registry" } },
                    spec: {
                        containers: [{
                            name: "registry",
                            image: "registry:2",
                            ports: [{ containerPort: 5000 }],
                            volumeMounts: [{
                                name: "registry-storage",
                                mountPath: "/var/lib/registry"
                            }]
                        }],
                        volumes: [{
                            name: "registry-storage",
                            persistentVolumeClaim: {
                                claimName: this.pvc.metadata.name,
                            }
                        }]
                    }
                }
            }
        }, { parent: this });

        this.service = new k8s.core.v1.Service(name + "-svc", {
            metadata: { namespace: "zeitwork-system", name: name + "-svc" },
            spec: {
                selector: { app: "registry" },
                ports: [{ port: 5000, targetPort: 5000 }],
                type: "ClusterIP"
            }
        }, { parent: this });

        this.certificate = new k8s.apiextensions.CustomResource(name + "-cert", {
            apiVersion: "cert-manager.io/v1",
            kind: "Certificate",
            metadata: { namespace: "zeitwork-system", name: name + "-cert" },
            spec: {
                secretName: name + "-tls",
                issuerRef: { name: "letsencrypt", kind: "ClusterIssuer" },
                commonName: "registry.zeitwork.com",
                dnsNames: ["registry.zeitwork.com"],
            }
        }, { parent: this });

        this.proxy = new k8s.apiextensions.CustomResource(name + "-proxy", {
            apiVersion: "projectcontour.io/v1",
            kind: "HTTPProxy",
            metadata: { namespace: "zeitwork-system", name: name + "-proxy" },
            spec: {
                virtualhost: {
                    fqdn: "registry.zeitwork.com",
                    tls: {
                        secretName: name + "-tls"
                    }
                },
                routes: [{
                    conditions: [{ prefix: "/" }],
                    services: [{ name: this.service.metadata.name, port: 5000 }]
                }]
            }
        }, { parent: this, dependsOn: this.certificate });

        this.registerOutputs();
    }
}

