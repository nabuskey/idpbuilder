package localbuild

import (
	"context"
	"embed"

	"github.com/cnoe-io/idpbuilder/api/v1alpha1"
	"github.com/cnoe-io/idpbuilder/pkg/util"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// hardcoded secret name and namespace from what we have in the yaml installation file.
	giteaNamespace   = "gitea"
	giteaAdminSecret = "gitea-admin-secret"
)

//go:embed resources/gitea/k8s/*
var installGiteaFS embed.FS

func RawGiteaInstallResources() ([][]byte, error) {
	return util.ConvertFSToBytes(installGiteaFS, "resources/nginx/k8s")
}

func (r *LocalbuildReconciler) ReconcileGitea(ctx context.Context, req ctrl.Request, resource *v1alpha1.Localbuild) (ctrl.Result, error) {
	gitea := EmbeddedInstallation{
		name:         "Gitea",
		resourcePath: "resources/gitea/k8s",
		resourceFS:   installGiteaFS,
		namespace:    giteaNamespace,
		monitoredResources: map[string]schema.GroupVersionKind{
			"my-gitea": {
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
		},
	}

	if result, err := gitea.Install(ctx, req, resource, r.Client, r.Scheme); err != nil {
		return result, err
	}

	resource.Status.GiteaSecretName = giteaAdminSecret
	resource.Status.GiteaSecretNamespace = giteaNamespace
	resource.Status.GiteaAvailable = true
	return ctrl.Result{}, nil
}
