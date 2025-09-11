package firecracker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// CreateInstance ensures image availability and creates a container entry in containerd.
func (r *Runtime) CreateInstance(ctx context.Context, spec *types.InstanceSpec) (*types.Instance, error) {
	name := generateName(spec.ID)
	// Ensure image present in namespace via images pull/import. We prefer pulling here for simplicity.
	fullImage := r.ensureRegistry(spec.ImageTag)
	if err := r.pullImage(ctx, fullImage); err != nil {
		return nil, fmt.Errorf("pull image failed: %w", err)
	}

	inst := &types.Instance{
		ID:        spec.ID,
		ImageID:   spec.ImageID,
		ImageTag:  fullImage,
		State:     types.InstanceStateCreating,
		Resources: spec.Resources,
		EnvVars:   spec.EnvironmentVariables,
		CreatedAt: time.Now(),
		RuntimeID: name,
	}
	return inst, nil
}

func (r *Runtime) ensureRegistry(imageTag string) string {
	if strings.Contains(imageTag, "/") {
		return imageTag
	}
	if r.cfg.ImageRegistry != "" {
		return fmt.Sprintf("%s/%s", r.cfg.ImageRegistry, imageTag)
	}
	return imageTag
}

func (r *Runtime) pullImage(ctx context.Context, image string) error {
	// If the image already exists in the namespace, skip pulling to support pre-imported images
	if exists, err := r.imageExists(ctx, image); err == nil && exists {
		return nil
	}
	args := []string{"images", "pull"}
	args = append(args, "--snapshotter", "devmapper")
	args = append(args, image)
	if _, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, args); err != nil {
		return err
	}
	return nil
}

// imageExists returns true if the image is already present in the namespace
func (r *Runtime) imageExists(ctx context.Context, image string) (bool, error) {
	refs, err := r.listImageRefs(ctx)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		if ref == image || strings.HasSuffix(ref, "/"+image) {
			return true, nil
		}
		// Handle common default registry prefixes
		if strings.HasPrefix(ref, "docker.io/") && strings.TrimPrefix(ref, "docker.io/") == image {
			return true, nil
		}
		if strings.HasPrefix(ref, "index.docker.io/") && strings.TrimPrefix(ref, "index.docker.io/") == image {
			return true, nil
		}
		if strings.HasPrefix(ref, "registry-1.docker.io/") && strings.TrimPrefix(ref, "registry-1.docker.io/") == image {
			return true, nil
		}
	}
	return false, nil
}

// resolveImageRef attempts to find a fully-qualified image reference in containerd
// that matches the requested tag (handling default registry prefixes). Falls back
// to the requested value if no better match is found.
func (r *Runtime) resolveImageRef(ctx context.Context, requested string) (string, error) {
	refs, err := r.listImageRefs(ctx)
	if err != nil {
		return requested, nil
	}
	requested = strings.TrimSpace(requested)
	var candidates []string
	candidates = append(candidates, requested)
	if r.cfg.ImageRegistry != "" {
		candidates = append(candidates, r.cfg.ImageRegistry+"/"+requested)
	}
	candidates = append(candidates,
		"docker.io/"+requested,
		"index.docker.io/"+requested,
		"registry-1.docker.io/"+requested,
	)
	// Prefer exact match among known candidates
	set := make(map[string]bool)
	for _, ref := range refs {
		set[strings.TrimSpace(ref)] = true
	}
	for _, c := range candidates {
		if set[c] {
			return c, nil
		}
	}
	// Otherwise, match by suffix to handle nested namespaces
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == requested || strings.HasSuffix(ref, "/"+requested) {
			return ref, nil
		}
	}
	return requested, nil
}

// listImageRefs returns the REF column from `firecracker-ctr images list`
func (r *Runtime) listImageRefs(ctx context.Context) ([]string, error) {
	out, err := runFCNS(ctx, r.cfg, r.cfg.ContainerdNamespace, []string{"images", "list"})
	if err != nil {
		return nil, err
	}
	var refs []string
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) <= 1 {
		return refs, nil
	}
	for _, ln := range lines[1:] { // skip header
		fields := strings.Fields(ln)
		if len(fields) >= 1 {
			refs = append(refs, fields[0])
		}
	}
	return refs, nil
}
