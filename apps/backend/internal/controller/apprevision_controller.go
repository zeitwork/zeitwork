/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"github.com/zeitwork/zeitwork/internal/services"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	projectcontourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/zeitwork/zeitwork/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AppRevisionReconciler reconciles a AppRevision object
type AppRevisionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Services *services.Services
}

// +kubebuilder:rbac:groups=zeitwork.com,resources=apprevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zeitwork.com,resources=apprevisions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zeitwork.com,resources=apprevisions/finalizers,verbs=update
// +kubebuilder:rbac:groups=zeitwork.com,resources=apps,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=projectcontour.io,resources=httpproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete

// getOwnerApp returns the owner App if it exists, otherwise returns nil
func (r *AppRevisionReconciler) getOwnerApp(ctx context.Context, revision *v1alpha1.AppRevision) (*v1alpha1.App, error) {
	for _, ownerRef := range revision.OwnerReferences {
		if ownerRef.Kind == "App" && ownerRef.APIVersion == "zeitwork.com/v1alpha1" {
			app := &v1alpha1.App{}
			if err := r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: revision.Namespace}, app); err != nil {
				return nil, err
			}
			return app, nil
		}
	}
	return nil, nil
}

// createBuildJob creates a job to build the image for the given commit SHA
func (r *AppRevisionReconciler) createBuildJob(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) error {
	jobName := fmt.Sprintf("build-%s", revision.Name)

	// get access token for the org
	token, err := r.Services.Github.GetTokenForInstallation(ctx, app.Spec.GithubInstallation)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token for installation %d: %w", app.Spec.GithubInstallation, err)
	}

	// Determine build context and dockerfile paths based on BasePath
	contextPath := "/workspace"
	dockerfilePath := "/workspace"

	if app.Spec.BasePath != nil && *app.Spec.BasePath != "" {
		contextPath = fmt.Sprintf("/workspace/%s", *app.Spec.BasePath)
		dockerfilePath = contextPath
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: revision.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					//RuntimeClassName: pointer.String("gvisor"),
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "clone-repo",
							Image:   "alpine/git:latest",
							Command: []string{"sh", "-c"},
							Args: []string{
								fmt.Sprintf("git clone https://x-access-token:%s@github.com/%s/%s.git /workspace && cd /workspace && git checkout %s && chown -R 1000:1000 /workspace", token, app.Spec.GithubOwner, app.Spec.GithubRepo, revision.Spec.CommitSHA),
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}},
						}},
					Containers: []corev1.Container{
						{
							Name:  "builder",
							Image: "moby/buildkit:v0.23.2",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
							},
							Command: []string{"/usr/bin/buildctl-daemonless.sh"},
							Args: []string{
								"build",
								"--frontend=dockerfile.v0",
								fmt.Sprintf("--local=context=%s", contextPath),
								fmt.Sprintf("--local=dockerfile=%s", dockerfilePath),
								fmt.Sprintf("--output=type=image,name=registry.zeitwork.com/%s:%s,push=true", revision.Name, revision.Spec.CommitSHA[:7]),
								"--progress=plain",
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: pointer.Bool(true), // todo: this should be fixed
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(revision, job, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, job)
}

// checkBuildJobStatus checks if the build job is complete and updates ImageBuilt status
func (r *AppRevisionReconciler) checkBuildJobStatus(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) (bool, error) {
	jobName := fmt.Sprintf("build-%s", revision.Name)
	job := &batchv1.Job{}

	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: revision.Namespace}, job); err != nil {
		return false, client.IgnoreNotFound(err)
	}

	// Check if job is complete
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			// Job completed successfully, update ImageBuilt status
			imageTag := fmt.Sprintf("registry.zeitwork.com/%s:%s", revision.Name, revision.Spec.CommitSHA[:7])
			revision.Status.ImageBuilt = &imageTag
			return true, r.Status().Update(ctx, revision)
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return false, fmt.Errorf("build job failed")
		}
	}

	// Job is still running
	return false, nil
}

// ensureDeployment creates or updates the deployment with the built image
func (r *AppRevisionReconciler) ensureDeployment(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) error {
	if revision.Status.ImageBuilt == nil {
		return nil // Image not built yet
	}

	deploymentName := fmt.Sprintf("%s-deployment", revision.Name)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: revision.Namespace,
		},
	}

	probe := &corev1.Probe{
		InitialDelaySeconds: 5,
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/",
				Port: intstr.IntOrString{
					IntVal: app.Spec.Port,
				},
			},
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, deployment, func() error {
		deployment.Spec = appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": app.Name, "revision": revision.Name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app.Name, "revision": revision.Name}},
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("gvisor"),
					Containers: []corev1.Container{
						{
							Name:           "app-container",
							Image:          *revision.Status.ImageBuilt,
							Ports:          []corev1.ContainerPort{{Protocol: corev1.ProtocolTCP, ContainerPort: app.Spec.Port}},
							ReadinessProbe: probe,
							LivenessProbe:  probe,
							Env:            app.Spec.Env,
						},
					},
				},
			},
			Replicas: pointer.Int32(1),
		}

		return controllerutil.SetControllerReference(revision, deployment, r.Scheme)
	})
	if err != nil {
		return err
	}

	// if the deployment has at least one readyReplicas, we can set appRevision to ready
	if deployment.Status.ReadyReplicas > 0 {
		_, err = controllerutil.CreateOrPatch(ctx, r.Client, revision, func() error {
			revision.Status.Ready = true
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// ensureService creates or updates a Service pointing to the deployment
func (r *AppRevisionReconciler) ensureService(ctx context.Context, app *v1alpha1.App, revision *v1alpha1.AppRevision, deployment *appsv1.Deployment) error {
	serviceName := fmt.Sprintf("svc-%s", revision.Name)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: revision.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app":      app.Name,
				"revision": revision.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     app.Spec.Port,
					TargetPort: intstr.IntOrString{
						IntVal: app.Spec.Port,
					},
				},
			},
		}
		return controllerutil.SetControllerReference(revision, service, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

// ensureHTTPProxy creates or updates a Contour HTTPProxy resource
func (r *AppRevisionReconciler) ensureHTTPProxy(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) error {
	httpProxyName := fmt.Sprintf("%s-proxy", revision.Name)
	httpProxy := &projectcontourv1.HTTPProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpProxyName,
			Namespace: revision.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, httpProxy, func() error {
		certName := fmt.Sprintf("%s-cert", revision.Name)
		fqdn := fmt.Sprintf("%s-%s.zeitwork.app", revision.Spec.CommitSHA[:7], app.Name)

		httpProxy.Spec.VirtualHost = &projectcontourv1.VirtualHost{
			Fqdn: fqdn,
			TLS: &projectcontourv1.TLS{
				SecretName: certName + "-tls",
			},
		}
		httpProxy.Spec.Routes = []projectcontourv1.Route{
			{
				Services: []projectcontourv1.Service{
					{
						Name: fmt.Sprintf("svc-%s", revision.Name),
						Port: int(app.Spec.Port),
					},
				},
			},
		}
		return controllerutil.SetControllerReference(revision, httpProxy, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

// ensureCertificate creates or updates a cert-manager Certificate for the AppRevision
func (r *AppRevisionReconciler) ensureCertificate(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) error {
	certName := fmt.Sprintf("%s-cert", revision.Name)
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: revision.Namespace,
		},
	}

	fqdn := fmt.Sprintf("%s-%s.zeitwork.app", revision.Spec.CommitSHA[:7], app.Name)
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, func() error {
		cert.Spec = certmanagerv1.CertificateSpec{
			SecretName: certName + "-tls",
			CommonName: fqdn,
			DNSNames:   []string{fqdn},
			IssuerRef: cmmeta.ObjectReference{
				Name:  "letsencrypt",
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				RotationPolicy: certmanagerv1.RotationPolicyAlways,
			},
		}
		return controllerutil.SetControllerReference(revision, cert, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AppRevisionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AppRevision instance
	revision := &v1alpha1.AppRevision{}
	if err := r.Get(ctx, req.NamespacedName, revision); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if owner reference is set, return if not
	app, err := r.getOwnerApp(ctx, revision)
	if err != nil {
		logger.Error(err, "failed to get owner app")
		return ctrl.Result{}, err
	}
	if app == nil {
		logger.Info("no owner app found, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// ensure app is labeled
	patch := client.MergeFrom(revision.DeepCopy())
	if revision.Labels == nil {
		revision.Labels = map[string]string{}
	}
	revision.Labels["zeitwork.com/app"] = app.Name
	err = r.Patch(ctx, revision, patch)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure TLS certificate for the HTTPProxy
	if err := r.ensureCertificate(ctx, revision, app); err != nil {
		logger.Error(err, "failed to ensure certificate")
		return ctrl.Result{}, err
	}

	// Check if ImageBuilt is set
	if revision.Status.ImageBuilt == nil {
		// Image not built yet, check if build job exists
		jobComplete, err := r.checkBuildJobStatus(ctx, revision, app)
		if err != nil {
			logger.Error(err, "failed to check build job status")
			return ctrl.Result{}, err
		}

		if !jobComplete {
			// Job doesn't exist or is still running, create it if needed
			jobName := fmt.Sprintf("build-%s", revision.Name)
			job := &batchv1.Job{}
			if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: revision.Namespace}, job); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			} else if err != nil {
				// Job doesn't exist, create it
				if err := r.createBuildJob(ctx, revision, app); err != nil {
					logger.Error(err, "failed to create build job")
					return ctrl.Result{}, err
				}
			}

			// Return here, we watch the job so reconciliation will continue once the job is complete
			return ctrl.Result{}, nil
		}
	}

	// Ensure deployment is created and has the correct image
	if err := r.ensureDeployment(ctx, revision, app); err != nil {
		logger.Error(err, "failed to ensure deployment")
		return ctrl.Result{}, err
	}

	if err := r.ensureService(ctx, app, revision, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-deployment", revision.Name),
			Namespace: revision.Namespace,
		},
	}); err != nil {
		logger.Error(err, "failed to ensure service")
		return ctrl.Result{}, err
	}

	if err := r.ensureHTTPProxy(ctx, revision, app); err != nil {
		logger.Error(err, "failed to ensure HTTPProxy")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppRevisionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AppRevision{}).
		Owns(&batchv1.Job{}).
		Owns(&appsv1.Deployment{}).
		Watches(&v1alpha1.App{}, handler.EnqueueRequestsFromMapFunc(r.findAppParents)).
		Complete(r)
}

// findAppParents returns a list of AppRevision reconcile requests for the given App
func (r *AppRevisionReconciler) findAppParents(ctx context.Context, app client.Object) []ctrl.Request {
	var requests []ctrl.Request

	// List all AppRevisions in the same namespace as the App
	appRevisions := &v1alpha1.AppRevisionList{}
	if err := r.List(ctx, appRevisions, client.InNamespace(app.GetNamespace())); err != nil {
		return requests
	}

	// Find AppRevisions that are owned by this App
	for _, revision := range appRevisions.Items {
		for _, ownerRef := range revision.OwnerReferences {
			if ownerRef.Kind == "App" && ownerRef.APIVersion == "zeitwork.com/v1alpha1" && ownerRef.Name == app.GetName() {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      revision.Name,
						Namespace: revision.Namespace,
					},
				})
				break
			}
		}
	}

	return requests
}
