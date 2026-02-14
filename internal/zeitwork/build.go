package zeitwork

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/jackc/pgx/v5"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileBuild(ctx context.Context, objectID uuid.UUID) error {
	if !s.isControlPlaneLeader() {
		return nil
	}

	build, err := s.db.BuildClaimLease(ctx, queries.BuildClaimLeaseParams{
		ID:           objectID,
		ProcessingBy: s.serverID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another server currently owns the lease, lease is stale-unclaimable yet,
		// or the build is already terminal/deleted.
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to claim build lease: %w", err)
	}
	defer s.releaseBuildLease(objectID)

	// Skip if already completed
	if build.Status == queries.BuildStatusSuccesful || build.Status == queries.BuildStatusFailed {
		return nil
	}

	// Check for stuck builds - if building for more than 15 minutes, mark as failed
	if build.Status == queries.BuildStatusBuilding && build.BuildingAt.Valid {
		buildingDuration := time.Since(build.BuildingAt.Time)
		if buildingDuration > 15*time.Minute {
			slog.Warn("build stuck, marking as failed", "build_id", build.ID, "building_for", buildingDuration)
			return s.db.BuildMarkFailed(ctx, build.ID)
		}
	}

	// Check if this build is already being executed (prevent concurrent execution)
	s.activeBuildsMu.Lock()
	if s.activeBuilds[build.ID] {
		s.activeBuildsMu.Unlock()
		slog.Debug("build already being executed, skipping", "build_id", build.ID)
		return nil
	}
	s.activeBuilds[build.ID] = true
	s.activeBuildsMu.Unlock()

	// Ensure we remove from active builds when done
	defer func() {
		s.activeBuildsMu.Lock()
		delete(s.activeBuilds, build.ID)
		s.activeBuildsMu.Unlock()
	}()

	// reconcile build image for build vm (atomic find-or-create to avoid race conditions)
	buildImage, err := s.db.ImageFindOrCreate(ctx, queries.ImageFindOrCreateParams{
		ID:         uuid.New(),
		Registry:   "ghcr.io",
		Repository: "tomhaerter/dind",
		Tag:        "latest",
	})
	if err != nil {
		return err
	}

	// if we dont have a vm create one
	if !build.VmID.Valid {
		vm, err := s.VMCreate(ctx, VMCreateParams{
			VCPUs:   2,
			Memory:  4 * 1024,
			ImageID: buildImage.ID,
			Port:    2375,
		})
		if err != nil {
			return err
		}
		slog.Info("created build VM", "build_id", build.ID, "vm_id", vm.ID)
		// Only mark as building if currently pending (avoid unnecessary DB update)
		if build.Status == queries.BuildStatusPending {
			return s.db.BuildMarkBuilding(ctx, queries.BuildMarkBuildingParams{
				ID:   build.ID,
				VmID: vm.ID,
			})
		}
		return nil
	}

	// We have a VM, check if it's ready
	vm, err := s.db.VMFirstByID(ctx, build.VmID)
	if err != nil {
		return err
	}

	// If VM failed, fail the build (only if not already failed)
	if vm.Status == queries.VmStatusFailed {
		slog.Error("build VM failed", "build_id", build.ID, "vm_id", vm.ID)
		if build.Status != queries.BuildStatusFailed {
			s.db.BuildMarkFailed(ctx, build.ID)
		}
		return nil
	}

	// VM not running yet, wait
	if vm.Status != queries.VmStatusRunning {
		slog.Info("build VM not ready yet", "build_id", build.ID, "vm_id", vm.ID, "status", vm.Status)
		return nil
	}

	// VM is running and we don't have an output image yet - execute the build
	if !build.ImageID.Valid {
		slog.Info("starting build execution", "build_id", build.ID, "vm_id", vm.ID)
		err = s.executeBuild(ctx, build, vm)
		if err != nil {
			slog.Error("build execution failed", "build_id", build.ID, "error", err)
			// Only mark as failed if not already failed (avoid unnecessary DB update)
			if build.Status != queries.BuildStatusFailed {
				s.db.BuildMarkFailed(ctx, build.ID)
			}
			s.cleanupBuildVM(ctx, build.VmID)
			return nil // Don't return error - we've handled it by marking failed
		}
	}

	return nil
}

func (s *Service) releaseBuildLease(buildID uuid.UUID) {
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.db.BuildReleaseLease(releaseCtx, queries.BuildReleaseLeaseParams{
		ID:           buildID,
		ProcessingBy: s.serverID,
	}); err != nil {
		slog.Warn("failed to release build lease", "build_id", buildID, "server_id", s.serverID, "err", err)
	}
}

// executeBuild performs the actual build process inside the VM
func (s *Service) executeBuild(ctx context.Context, build queries.Build, vm queries.Vm) error {
	// 1. Get project info
	project, err := s.db.ProjectFirstByID(ctx, build.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// 2. Check if GitHub token service is available
	if s.githubTokenService == nil {
		return fmt.Errorf("github token service not configured")
	}

	// 3. Get GitHub installation token
	token, err := s.githubTokenService.GetInstallationToken(ctx, project.GithubInstallationID)
	if err != nil {
		return fmt.Errorf("failed to get github token: %w", err)
	}

	slog.Info("got github token", "build_id", build.ID, "repo", project.GithubRepository)

	// 4. Download source tarball from GitHub
	tarballReader, err := s.downloadSourceTarball(ctx, token, project.GithubRepository, build.GithubCommit)
	if err != nil {
		return fmt.Errorf("failed to download source: %w", err)
	}
	defer tarballReader.Close()

	slog.Info("downloaded source tarball", "build_id", build.ID)

	// 5. Connect to Docker daemon in VM
	dockerHost := fmt.Sprintf("tcp://%s:2375", vm.IpAddress.Addr())
	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to docker: %w", err)
	}
	defer dockerClient.Close()

	// Wait for Docker daemon to be ready
	if err := s.waitForDockerReady(ctx, dockerClient); err != nil {
		return fmt.Errorf("docker daemon not ready: %w", err)
	}

	slog.Info("connected to docker daemon", "build_id", build.ID, "host", dockerHost)

	// 6. Prepare build context from tarball (convert gzip tarball to Docker-compatible tar)
	buildContext, err := s.prepareBuildContext(tarballReader, project.RootDirectory)
	if err != nil {
		return fmt.Errorf("failed to prepare build context: %w", err)
	}

	// 7. Build the image
	imageTag := fmt.Sprintf("%s/%s/%s:%s",
		s.cfg.DockerRegistryURL,                 // e.g., "ghcr.io"
		s.cfg.DockerRegistryUsername,            // e.g., "zeitwork"
		hex.EncodeToString(project.ID.Bytes[:]), // unique project identifier (lowercase hex)
		build.GithubCommit,                      // commit sha as tag
	)

	slog.Info("building docker image with buildx", "build_id", build.ID, "tag", imageTag)

	// Build and push using docker buildx with OCI media types
	// We run a container with the docker CLI to execute buildx build
	if err := s.runBuildxBuild(ctx, dockerClient, build, buildContext, imageTag); err != nil {
		return fmt.Errorf("buildx build failed: %w", err)
	}

	slog.Info("docker build and push completed", "build_id", build.ID)

	// 9. Create image record in DB (use find-or-create to handle concurrent builds for same commit)
	outputImage, err := s.db.ImageFindOrCreate(ctx, queries.ImageFindOrCreateParams{
		ID:         uuid.New(),
		Registry:   s.cfg.DockerRegistryURL,
		Repository: fmt.Sprintf("%s/%s", s.cfg.DockerRegistryUsername, hex.EncodeToString(project.ID.Bytes[:])),
		Tag:        build.GithubCommit,
	})
	if err != nil {
		return fmt.Errorf("failed to find or create image record: %w", err)
	}

	slog.Info("created output image record", "build_id", build.ID, "image_id", outputImage.ID)

	// 10. Mark build as successful (only if not already successful)
	if build.Status != queries.BuildStatusSuccesful {
		err = s.db.BuildMarkSuccessful(ctx, queries.BuildMarkSuccessfulParams{
			ID:      build.ID,
			ImageID: outputImage.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to mark build successful: %w", err)
		}
	}

	slog.Info("build completed successfully", "build_id", build.ID)

	// 11. Cleanup VM
	s.cleanupBuildVM(ctx, build.VmID)

	return nil
}

// downloadSourceTarball downloads the source code as a tarball from GitHub
func (s *Service) downloadSourceTarball(ctx context.Context, token, repo, commit string) (io.ReadCloser, error) {
	// GitHub tarball URL format: https://api.github.com/repos/{owner}/{repo}/tarball/{ref}
	url := fmt.Sprintf("https://api.github.com/repos/%s/tarball/%s", repo, commit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("github returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// prepareBuildContext converts a GitHub gzip tarball to a Docker build context
// GitHub tarballs have a top-level directory like "owner-repo-sha/" that we need to strip
// rootDir specifies the subdirectory to use as build context (e.g., "/" for repo root, "/apps/web" for monorepo)
func (s *Service) prepareBuildContext(gzipReader io.Reader, rootDir string) (io.Reader, error) {
	// Create a pipe for streaming the output
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// Decompress gzip
		gzr, err := gzip.NewReader(gzipReader)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to create gzip reader: %w", err))
			return
		}
		defer gzr.Close()

		// Read input tar
		tr := tar.NewReader(gzr)

		// Write output tar
		tw := tar.NewWriter(pw)
		defer tw.Close()

		var githubPrefix string
		foundDockerfile := false

		// Normalize rootDir: remove leading slash for path matching, ensure trailing slash
		// "/" -> "" (repo root)
		// "/apps/web" -> "apps/web/"
		rootDirPrefix := strings.TrimPrefix(rootDir, "/")
		if rootDirPrefix != "" && !strings.HasSuffix(rootDirPrefix, "/") {
			rootDirPrefix = rootDirPrefix + "/"
		}

		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				pw.CloseWithError(fmt.Errorf("failed to read tar: %w", err))
				return
			}

			// Determine the GitHub prefix from the first entry (e.g., "owner-repo-sha/")
			if githubPrefix == "" {
				parts := strings.SplitN(header.Name, "/", 2)
				if len(parts) > 1 {
					githubPrefix = parts[0] + "/"
				}
			}

			// Strip the GitHub prefix first
			pathAfterGitHub := strings.TrimPrefix(header.Name, githubPrefix)
			if pathAfterGitHub == "" {
				continue // Skip the root directory itself
			}

			// If rootDir is specified, filter to only include files under that directory
			if rootDirPrefix != "" {
				// Skip files not under the root directory
				if !strings.HasPrefix(pathAfterGitHub, rootDirPrefix) {
					// Allow the root directory entry itself
					if pathAfterGitHub != strings.TrimSuffix(rootDirPrefix, "/") {
						continue
					}
				}
				// Strip the root directory prefix
				pathAfterGitHub = strings.TrimPrefix(pathAfterGitHub, rootDirPrefix)
				if pathAfterGitHub == "" {
					continue // Skip the root directory entry itself
				}
			}

			// Check if we found a Dockerfile at the root of the build context
			if filepath.Base(pathAfterGitHub) == "Dockerfile" && !strings.Contains(pathAfterGitHub, "/") {
				foundDockerfile = true
			}

			// Create new header with the final stripped name
			newHeader := *header
			newHeader.Name = pathAfterGitHub

			if err := tw.WriteHeader(&newHeader); err != nil {
				pw.CloseWithError(fmt.Errorf("failed to write tar header: %w", err))
				return
			}

			if header.Typeflag == tar.TypeReg {
				if _, err := io.Copy(tw, tr); err != nil {
					pw.CloseWithError(fmt.Errorf("failed to copy tar content: %w", err))
					return
				}
			}
		}

		if !foundDockerfile {
			if rootDirPrefix != "" {
				pw.CloseWithError(fmt.Errorf("no Dockerfile found in %s", rootDir))
			} else {
				pw.CloseWithError(fmt.Errorf("no Dockerfile found in repository root"))
			}
			return
		}
	}()

	return pr, nil
}

// waitForDockerReady waits for the Docker daemon to be ready
func (s *Service) waitForDockerReady(ctx context.Context, dockerClient *client.Client) error {
	for i := 0; i < 30; i++ { // Wait up to 30 seconds
		_, err := dockerClient.Ping(ctx)
		if err == nil {
			return nil
		}
		slog.Debug("waiting for docker daemon", "attempt", i+1, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return fmt.Errorf("docker daemon did not become ready")
}

// streamBuildLogs reads and logs the Docker build output, checking for errors
func (s *Service) streamBuildLogs(ctx context.Context, build queries.Build, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	// Docker build output can have very long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse Docker build output (JSON format)
		var msg struct {
			Stream      string `json:"stream"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}

		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not JSON, log as-is
			slog.Debug("build output", "build_id", build.ID, "line", line)
			continue
		}

		if msg.Error != "" {
			return fmt.Errorf("build error: %s", msg.Error)
		}

		if msg.Stream != "" {
			streamLine := strings.TrimSuffix(msg.Stream, "\n")
			if streamLine != "" {
				slog.Debug("build output", "build_id", build.ID, "stream", streamLine)

				// Store log in database
				s.db.BuildLogCreate(ctx, queries.BuildLogCreateParams{
					ID:             uuid.New(),
					BuildID:        build.ID,
					Message:        streamLine,
					Level:          "info",
					OrganisationID: build.OrganisationID,
				})
			}
		}
	}

	return scanner.Err()
}

// logWriter writes Docker log output line-by-line to the database in real-time
type logWriter struct {
	ctx   context.Context
	s     *Service
	build queries.Build
	level string
	buf   []byte
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)

	// Process complete lines
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}

		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]

		if line = strings.TrimSpace(line); line != "" {
			w.s.db.BuildLogCreate(w.ctx, queries.BuildLogCreateParams{
				ID:             uuid.New(),
				BuildID:        w.build.ID,
				Message:        line,
				Level:          w.level,
				OrganisationID: w.build.OrganisationID,
			})
		}
	}

	return len(p), nil
}

// Flush writes any remaining buffered content
func (w *logWriter) Flush() {
	if line := strings.TrimSpace(string(w.buf)); line != "" {
		w.s.db.BuildLogCreate(w.ctx, queries.BuildLogCreateParams{
			ID:             uuid.New(),
			BuildID:        w.build.ID,
			Message:        line,
			Level:          w.level,
			OrganisationID: w.build.OrganisationID,
		})
	}
	w.buf = nil
}

// runBuildxBuild executes docker buildx build inside a container to build and push with OCI media types
func (s *Service) runBuildxBuild(ctx context.Context, dockerClient *client.Client, build queries.Build, buildContext io.Reader, imageTag string) error {
	// Read build context into memory (needed to copy to container)
	buildContextBytes, err := io.ReadAll(buildContext)
	if err != nil {
		return fmt.Errorf("failed to read build context: %w", err)
	}

	// Create a container with docker CLI to run buildx
	// Using docker:cli image which has docker and buildx
	containerConfig := &container.Config{
		Image:      "docker:cli",
		WorkingDir: "/build",
		Env: []string{
			"DOCKER_HOST=unix:///var/run/docker.sock",
		},
		Cmd: []string{
			"docker", "buildx", "build",
			"--push",
			"--build-arg", "ZEITWORK=1",
			"--output", "type=image,oci-mediatypes=true",
			"-t", imageTag,
			"-f", "Dockerfile",
			".",
		},
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: "/var/run/docker.sock",
				Target: "/var/run/docker.sock",
			},
		},
	}

	// Pull the docker:cli image first (in case it's not present)
	slog.Info("pulling docker:cli image", "build_id", build.ID)
	pullResp, err := dockerClient.ImagePull(ctx, "docker:cli", image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull docker:cli image: %w", err)
	}
	io.Copy(io.Discard, pullResp)
	pullResp.Close()

	// Create the container
	slog.Info("creating builder container", "build_id", build.ID)
	createResp, err := dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create builder container: %w", err)
	}
	containerID := createResp.ID

	// Ensure cleanup
	defer func() {
		dockerClient.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	}()

	// Copy build context into container at /build
	slog.Info("copying build context to container", "build_id", build.ID, "size", len(buildContextBytes))
	err = dockerClient.CopyToContainer(ctx, containerID, "/build", bytes.NewReader(buildContextBytes), container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("failed to copy build context: %w", err)
	}

	// Configure registry auth for buildx by creating docker config
	dockerConfigJSON := fmt.Sprintf(`{"auths":{"%s":{"username":"%s","password":"%s"}}}`,
		s.cfg.DockerRegistryURL,
		s.cfg.DockerRegistryUsername,
		s.cfg.DockerRegistryPAT,
	)

	// Create a tar archive with the docker config (include .docker directory in path)
	var configTar bytes.Buffer
	tw := tar.NewWriter(&configTar)
	configBytes := []byte(dockerConfigJSON)
	tw.WriteHeader(&tar.Header{
		Name: ".docker/config.json",
		Mode: 0600,
		Size: int64(len(configBytes)),
	})
	tw.Write(configBytes)
	tw.Close()

	// Copy docker config to /root (tar will create .docker/ subdirectory)
	err = dockerClient.CopyToContainer(ctx, containerID, "/root", &configTar, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("failed to copy docker config: %w", err)
	}

	// Start the container
	slog.Info("starting buildx build", "build_id", build.ID, "tag", imageTag, "container_id", containerID)
	if err := dockerClient.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start builder container: %w", err)
	}

	slog.Info("container started, streaming logs in real-time", "build_id", build.ID, "container_id", containerID)

	// Start streaming logs in real-time (Follow: true)
	logReader, err := dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true, // Stream logs as they're written
	})
	if err != nil {
		slog.Error("failed to attach to container logs", "build_id", build.ID, "error", err)
		return fmt.Errorf("failed to attach to container logs: %w", err)
	}

	// Create writers that insert logs to database in real-time
	stdoutWriter := &logWriter{ctx: ctx, s: s, build: build, level: "info"}
	stderrWriter := &logWriter{ctx: ctx, s: s, build: build, level: "error"}

	// Stream logs in a goroutine
	var wg sync.WaitGroup
	var logErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer logReader.Close()
		// stdcopy.StdCopy demuxes docker's multiplexed stdout/stderr stream
		_, logErr = stdcopy.StdCopy(stdoutWriter, stderrWriter, logReader)
		// Flush any remaining partial lines
		stdoutWriter.Flush()
		stderrWriter.Flush()
	}()

	// Wait for container to finish
	statusCh, errCh := dockerClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	var exitCode int64
	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("error waiting for container", "build_id", build.ID, "error", err)
			return fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
		slog.Info("container exited", "build_id", build.ID, "exit_code", exitCode)
	}

	// Wait for log streaming to complete
	wg.Wait()
	if logErr != nil {
		slog.Warn("error streaming logs", "build_id", build.ID, "error", logErr)
	}

	if exitCode != 0 {
		return fmt.Errorf("buildx build failed with exit code %d", exitCode)
	}

	slog.Info("buildx build completed successfully", "build_id", build.ID)
	return nil
}

// cleanupBuildVM marks the build VM for deletion
func (s *Service) cleanupBuildVM(ctx context.Context, vmID uuid.UUID) {
	slog.Info("cleaning up build VM", "vm_id", vmID)

	// Soft delete the VM - the VM reconciler will handle actual cleanup
	if err := s.db.VMSoftDelete(ctx, vmID); err != nil {
		slog.Error("failed to soft delete build VM", "vm_id", vmID, "error", err)
	}
}
