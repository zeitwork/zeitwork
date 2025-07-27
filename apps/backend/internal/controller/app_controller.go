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
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	projectcontourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/zeitwork/zeitwork/internal/services"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/zeitwork/zeitwork/api/v1alpha1"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Services *services.Services
}

// +kubebuilder:rbac:groups=zeitwork.com,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zeitwork.com,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zeitwork.com,resources=apps/finalizers,verbs=update
// +kubebuilder:rbac:groups=zeitwork.com,resources=apprevisions,verbs=get;list;watch;create;update;patch;delete

// getDesiredRevisionName returns the expected AppRevision name and SHA
func getDesiredRevisionName(app *v1alpha1.App) (string, string, bool) {
	if app.Spec.DesiredRevisionSHA == nil {
		return "", "", false
	}
	sha := *app.Spec.DesiredRevisionSHA
	if len(sha) < 7 {
		return "", sha, false
	}
	name := app.Name + "-" + sha[:7]
	return name, sha, true
}

// ensureAppRevision creates or updates the AppRevision for the desired SHA
func (r *AppReconciler) ensureAppRevision(ctx context.Context, app *v1alpha1.App, name, sha string) error {
	revision := &v1alpha1.AppRevision{}
	err := r.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: name}, revision)

	// if the AppRevision does not exist, create it
	if apierrors.IsNotFound(err) {
		revision = &v1alpha1.AppRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: app.Namespace,
			},
			Spec: v1alpha1.AppRevisionSpec{
				CommitSHA: sha,
			},
		}
		if err := ctrl.SetControllerReference(app, revision, r.Scheme); err != nil {
			return err
		}

		return r.Create(ctx, revision)
	}

	// Ensure owner reference
	if err := r.ensureOwnerRef(ctx, app, revision); err != nil {
		return err
	}

	return nil
}

func (r *AppReconciler) ensureProxy(ctx context.Context, app *v1alpha1.App, name, sha string) error {
	// if the app does not have a fqdn set, no proxy is required
	if app.Spec.FQDN == nil {
		return nil
	}

	revision := &v1alpha1.AppRevision{}
	err := r.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: name}, revision)
	if err != nil {
		return err
	}

	// if the revision is not ready, we cannot proceed.
	if !revision.Status.Ready {
		return nil
	}

	// ensure the cert
	err = r.ensureCertificate(ctx, revision, app)
	if err != nil {
		return err
	}

	// ensure the proxy
	httpProxy := &projectcontourv1.HTTPProxy{ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-proxy", app.Name),
		Namespace: revision.Namespace,
	}}

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, httpProxy, func() error {
		httpProxy.Spec.VirtualHost = &projectcontourv1.VirtualHost{
			Fqdn: *app.Spec.FQDN,
			TLS: &projectcontourv1.TLS{
				SecretName: fmt.Sprintf("%s-cert-tls", app.Name),
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
		return controllerutil.SetControllerReference(app, httpProxy, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *AppReconciler) ensureCertificate(ctx context.Context, revision *v1alpha1.AppRevision, app *v1alpha1.App) error {
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cert-tls", app.Name),
			Namespace: revision.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, func() error {
		cert.Spec = certmanagerv1.CertificateSpec{
			SecretName: fmt.Sprintf("%s-cert-tls", app.Name),
			CommonName: *app.Spec.FQDN,
			DNSNames:   []string{*app.Spec.FQDN},
			IssuerRef: cmmeta.ObjectReference{
				Name:  "letsencrypt",
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				RotationPolicy: certmanagerv1.RotationPolicyAlways,
			},
		}
		return controllerutil.SetControllerReference(app, cert, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

// ensureOwnerRef ensures the AppRevision has the correct owner reference
func (r *AppReconciler) ensureOwnerRef(ctx context.Context, app *v1alpha1.App, revision *v1alpha1.AppRevision) error {
	if !hasOwnerRef(app, revision) {
		if err := ctrl.SetControllerReference(app, revision, r.Scheme); err != nil {
			return err
		}
		return r.Update(ctx, revision)
	}
	return nil
}

// hasOwnerRef checks if the AppRevision has the correct owner reference
func hasOwnerRef(app *v1alpha1.App, revision *v1alpha1.AppRevision) bool {
	for _, ref := range revision.OwnerReferences {
		if ref.UID == app.UID && ref.Kind == "App" && ref.Name == app.Name {
			return true
		}
	}
	return false
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the App instance
	app := &v1alpha1.App{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if DesiredRevisionSHA is set and handle AppRevision. if there is no sha set, nothing to do.
	name, sha, ok := getDesiredRevisionName(app)
	if !ok {
		return ctrl.Result{}, nil
	}

	if err := r.ensureAppRevision(ctx, app, name, sha); err != nil {
		logger.Error(err, "failed to ensure AppRevision", "name", name, "sha", sha)
		return ctrl.Result{}, err
	}

	if err := r.ensureProxy(ctx, app, name, sha); err != nil {
		logger.Error(err, "failed to ensure Proxy", "name", name, "sha", sha)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.App{}).
		Owns(&v1alpha1.AppRevision{}).
		Complete(r)
}
