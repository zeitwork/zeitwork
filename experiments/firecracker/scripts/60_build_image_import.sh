#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "${SCRIPT_DIR}/common.sh"

NS=${NS:-fc6}
TAG=${1:-local/hello3000:latest}

tmp_dir=$(mktemp -d)
trap 'rm -rf ${tmp_dir}' EXIT

cat >"${tmp_dir}/Dockerfile" <<'EOF'
FROM busybox:latest
RUN mkdir -p /www && echo 'hello world' > /www/index.html
EXPOSE 3000
CMD ["busybox", "httpd", "-f", "-p", "[::]:3000", "-h", "/www"]
EOF

log "Building Docker image ${TAG}"
run "sudo docker build -t ${TAG} ${tmp_dir}"

tar_path="${tmp_dir}/image.tar"
log "Saving image to ${tar_path}"
run "sudo docker save ${TAG} -o ${tar_path}"

log "Importing image into firecracker-containerd (namespace: ${NS})"
run "sudo /usr/local/bin/firecracker-ctr --address /run/firecracker-containerd/containerd.sock -n ${NS} images import ${tar_path}"

log "Image imported"


