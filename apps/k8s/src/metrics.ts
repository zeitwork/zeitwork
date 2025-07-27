import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";
import * as random from "@pulumi/random";

export class Metrics extends pulumi.ComponentResource {
    public grafanaAdminPassword: pulumi.Output<string>;

    constructor(name: string, args: {}, opts?: pulumi.ComponentResourceOptions) {
        super("zeitwork:component:Hetzner", name, {}, opts);

        // Generate a random password for Grafana admin
        const grafanaPassword = new random.RandomPassword("grafana-admin-password", {
            length: 24,
            special: true,
        }, { parent: this });
        this.grafanaAdminPassword = grafanaPassword.result;

        const vmOperator = new k8s.helm.v3.Release("vm-operator", {
            namespace: "victoria",
            createNamespace: true,
            chart: "victoria-metrics-operator",
            repositoryOpts: {
                repo: "https://victoriametrics.github.io/helm-charts/",
            },
        }, { parent: this });

        new k8s.apiextensions.CustomResource("vmsingle", {
            apiVersion: "operator.victoriametrics.com/v1beta1",
            kind: "VMSingle",
            metadata: {
                "name": "vm-single",
                "namespace": "victoria"
            },
            spec: {
                disableSelfServiceScrape: true,
                storage: {
                    accessModes: [
                        "ReadWriteOnce"
                    ],
                    resources: {
                        requests: {
                            storage: "16Gi"
                        }
                    }
                },
                retentionPeriod: "4w",
            }
        }, {parent: this, dependsOn: [vmOperator]});

        new k8s.apiextensions.CustomResource("vmagent", {
            apiVersion: "operator.victoriametrics.com/v1beta1",
            kind: "VMAgent",
            metadata: {
                "name": "vm-agent",
                "namespace": "victoria"
            },
            spec: {
                useVMConfigReloader: true,
                scrapeInterval: "30s",
                remoteWrite: [{
                    url: "http://vmsingle-vm-single.victoria.svc.cluster.local:8428/api/v1/write"
                }],
                selectAllByDefault: true,
            }
        }, {parent: this, dependsOn: [vmOperator]});

        // Grafana Helm chart
        const grafana = new k8s.helm.v3.Release("grafana", {
            namespace: "victoria",
            chart: "grafana",
            repositoryOpts: {
                repo: "https://grafana.github.io/helm-charts",
            },
            values: {
                adminPassword: this.grafanaAdminPassword,
                persistence: {
                    enabled: true,
                    type: "pvc",
                    size: "2Gi",
                    accessModes: ["ReadWriteOnce"],
                },
                datasources: {
                    "datasources.yaml": {
                        apiVersion: 1,
                        datasources: [{
                            name: "VictoriaMetrics",
                            type: "prometheus",
                            url: "http://vmsingle-vm-single.victoria.svc.cluster.local:8428",
                            access: "proxy",
                            isDefault: true,
                        }]
                    }
                },
            }
        }, { parent: this, dependsOn: [vmOperator] });
    }
}

