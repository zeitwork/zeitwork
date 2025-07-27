import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";
import { Registry } from "./registry";
import {Hetzner} from "./hetzner";
import {Metrics} from "./metrics";

// Fetch kubeconfig from a Pulumi secret
const config = new pulumi.Config();
const kubeconfig = config.requireSecret("kubeconfig");

const provider = new k8s.Provider("k8s", {
    kubeconfig,
});

const namespace = new k8s.core.v1.Namespace("zeitwork-system", {
    metadata: {name: "zeitwork-system"},
}, {provider});

// raw yaml for the service monitor

const serviceMonitor = new k8s.yaml.v2.ConfigFile("service-monitor", {
    file: "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/refs/heads/main/example/prometheus-operator-crd-full/monitoring.coreos.com_servicemonitors.yaml"
}, {provider})

// projectcontour as ingress controller
const contour = new k8s.helm.v3.Release("contour", {
    chart: "contour",
    version: "21.0.9",
    repositoryOpts: {
        repo: "https://charts.bitnami.com/bitnami",
    },
    namespace: "zeitwork-contour",
    createNamespace: true,
    values: {
        configInline: {
            policy: {
                "response-headers": {
                    set: {
                        "X-Powered-By": "running on zeitwork.com"
                    }
                }
            }
        },
        envoy: {
            service: {
                type: 'NodePort',
            }
        },
        metrics: {
            serviceMonitor: {
                enabled: true
            }
        },
    }
}, {provider, dependsOn: [serviceMonitor]});

// cert-manager for SSL certificates
const certManager = new k8s.helm.v3.Release("cert-manager", {
    chart: "cert-manager",
    version: "1.18.2",
    repositoryOpts: {
        repo: "https://charts.jetstack.io",
    },
    namespace: "cert-manager",
    createNamespace: true,
    values: {
        installCRDs: true,
    },
}, {provider});

// ClusterIssuer for Let's Encrypt
const letsEncryptIssuer = new k8s.apiextensions.CustomResource("letsencrypt-issuer", {
    apiVersion: "cert-manager.io/v1",
    kind: "ClusterIssuer",
    metadata: { name: "letsencrypt" },
    spec: {
        acme: {
            server: "https://acme-v02.api.letsencrypt.org/directory",
            email: "hostmaster@zeitwork.com",
            privateKeySecretRef: { name: "letsencrypt-account-key" },
            solvers: [{
                http01: {
                    ingress: { class: "contour" }
                }
            }]
        }
    }
}, {provider, dependsOn: certManager});

// Deploy registry as a Pulumi component
const registry = new Registry("zeitwork-registry", {}, {
    provider,
});

const hetzner = new Hetzner("hetzner", {}, {
    provider
});

const metrics = new Metrics("victoria-metrics", {}, {
    provider
});

// Export Grafana admin password as a secret
export const grafanaAdminPassword = pulumi.secret(metrics.grafanaAdminPassword);

