import * as pulumi from "@pulumi/pulumi";
import * as k8s from "@pulumi/kubernetes";

export class Hetzner extends pulumi.ComponentResource {
    constructor(name: string, args: {}, opts?: pulumi.ComponentResourceOptions) {
        super("zeitwork:component:Hetzner", name, {}, opts);

        // hcloud-csi to dynamically provision volumes
        const hcloudCsi = new k8s.helm.v3.Release("hcloud-csi", {
            repositoryOpts: {
                repo: "https://charts.hetzner.cloud",
            },
            name: "hcloud-csi",
            chart: "hcloud-csi",
            namespace: "kube-system",
            values: {
                node: {
                    kubeletDir: '/var/lib/k0s/kubelet'
                }
            }
        }, { parent: this });
    }
}