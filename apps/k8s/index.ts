import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";
import { Registry } from "./registry";
import { Postgres } from "./postgres";

// Fetch kubeconfig from a Pulumi secret
const config = new pulumi.Config();
const kubeconfig = config.requireSecret("kubeconfig");

const provider = new k8s.Provider("k8s", {
    kubeconfig,
});

const namespace = new k8s.core.v1.Namespace("zeitwork-system", {
    metadata: {name: "zeitwork-system"},
}, {provider});

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
        envoy: {
            service: {
                type: 'NodePort',
            }
        }
    }
}, {provider});

// local-path-provisioner for persistent storage
new k8s.yaml.ConfigFile("local-path-storage", {
    file: "https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml",
}, {provider, dependsOn: contour});

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
const registry = new Registry("zeitwork-registry", {
    provider,
});

// Deploy Postgres as a Pulumi component
const postgres = new Postgres("zeitwork-postgres", {
    provider,
});