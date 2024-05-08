package custompackage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	argov1alpha1 "github.com/cnoe-io/argocd-api/api/argo/application/v1alpha1"
	"github.com/cnoe-io/idpbuilder/api/v1alpha1"
	"github.com/cnoe-io/idpbuilder/pkg/k8s"
	"github.com/cnoe-io/idpbuilder/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	requeueTime = time.Second * 30
)

type Reconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	Config   util.CorePackageTemplateConfig
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pkg := v1alpha1.CustomPackage{}
	err := r.Get(ctx, req.NamespacedName, &pkg)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.V(1).Info("reconciling custom package", "name", req.Name, "namespace", req.Namespace)
	defer r.postProcessReconcile(ctx, req, &pkg)
	result, err := r.reconcileCustomPackage(ctx, &pkg)
	if err != nil {
		r.Recorder.Event(&pkg, "Warning", "reconcile error", err.Error())
	} else {
		r.Recorder.Event(&pkg, "Normal", "reconcile success", "Successfully reconciled")
	}

	return result, err
}

func (r *Reconciler) postProcessReconcile(ctx context.Context, req ctrl.Request, pkg *v1alpha1.CustomPackage) {
	logger := log.FromContext(ctx)

	err := r.Status().Update(ctx, pkg)
	if err != nil {
		logger.Error(err, "failed updating repo status")
	}

	err = util.UpdateSyncAnnotation(ctx, r.Client, pkg)
	if err != nil {
		logger.Error(err, "failed updating repo annotation")
	}
}

// create an in-cluster repository CR, update the application spec, then apply
func (r *Reconciler) reconcileCustomPackage(ctx context.Context, resource *v1alpha1.CustomPackage) (ctrl.Result, error) {
	b, err := getArgoCDAppFile(ctx, resource)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reading file %s: %w", resource.Spec.ArgoCD.ApplicationFile, err)
	}

	var returnedRawResource []byte
	if returnedRawResource, err = util.ApplyTemplate(b, r.Config); err != nil {
		return ctrl.Result{}, err
	}

	objs, err := k8s.ConvertYamlToObjects(r.Scheme, returnedRawResource)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("converting yaml to object %w", err)
	}
	if len(objs) == 0 {
		return ctrl.Result{}, fmt.Errorf("file contained 0 kubernetes objects %s", resource.Spec.ArgoCD.ApplicationFile)
	}

	app, ok := objs[0].(*argov1alpha1.Application)
	if !ok {
		return ctrl.Result{}, fmt.Errorf("object is not an ArgoCD application %s", resource.Spec.ArgoCD.ApplicationFile)
	}

	appName := app.GetName()
	if resource.Spec.Replicate {
		synced := false
		repoRefs := make([]v1alpha1.ObjectRef, 0, 1)
		if app.Spec.HasMultipleSources() {
			for j := range app.Spec.Sources {
				s := &app.Spec.Sources[j]
				res, repo, sErr := r.reconcileArgoCDSource(ctx, resource, s, appName)
				if sErr != nil {
					return res, sErr
				}
				if repo != nil {
					synced = repo.Status.InternalGitRepositoryUrl != ""

					s.RepoURL = repo.Status.InternalGitRepositoryUrl
					repoRefs = append(repoRefs, v1alpha1.ObjectRef{
						Namespace: repo.Namespace,
						Name:      repo.Name,
						UID:       string(repo.ObjectMeta.UID),
					})
				}
			}
		} else {
			s := app.Spec.Source
			if isCNOEScheme(s.RepoURL) {
				res, repo, sErr := r.reconcileArgoCDSource(ctx, resource, s, appName)
				if sErr != nil {
					return res, sErr
				}
				if repo != nil {
					synced = repo.Status.InternalGitRepositoryUrl != ""
					s.RepoURL = repo.Status.InternalGitRepositoryUrl
					repoRefs = append(repoRefs, v1alpha1.ObjectRef{
						Namespace: repo.Namespace,
						Name:      repo.Name,
						UID:       string(repo.ObjectMeta.UID),
					})
				}
			}
		}
		resource.Status.GitRepositoryRefs = repoRefs
		resource.Status.Synced = synced
	}

	foundAppObj := argov1alpha1.Application{}
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(app), &foundAppObj)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Client.Create(ctx, app)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("creating %s app CR: %w", appName, err)
			}

			return ctrl.Result{RequeueAfter: requeueTime}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting argocd application object: %w", err)
	}

	foundAppObj.Spec = app.Spec
	foundAppObj.ObjectMeta.Annotations = app.Annotations
	foundAppObj.ObjectMeta.Labels = app.Labels
	err = r.Client.Update(ctx, &foundAppObj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("updating argocd application object %s: %w", appName, err)
	}
	return ctrl.Result{RequeueAfter: requeueTime}, nil
}

func (r *Reconciler) reconcileArgoCDSource(ctx context.Context, resource *v1alpha1.CustomPackage, appSource *argov1alpha1.ApplicationSource, appName string) (ctrl.Result, *v1alpha1.GitRepository, error) {
	if isCNOEScheme(appSource.RepoURL) {
		if resource.Spec.RemoteRepository.Url == "" {
			return r.reconcileArgoCDLocalSource(ctx, resource, appName, appSource.RepoURL)
		}
		return r.reconcileArgoCDRemoteSource(ctx, resource, appName, appSource.RepoURL)
	}
	return ctrl.Result{}, nil, nil
}

func (r *Reconciler) reconcileArgoCDRemoteSource(ctx context.Context, resource *v1alpha1.CustomPackage, appName, repoURL string) (ctrl.Result, *v1alpha1.GitRepository, error) {
	relativePath := strings.TrimPrefix(repoURL, v1alpha1.CNOEURIScheme)
	// no guarantee that this path exists
	dirPath := filepath.Join(resource.Spec.RemoteRepository.Path, relativePath)

	repo := &v1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteRepoName(appName, dirPath, resource.Spec.RemoteRepository),
			Namespace: resource.Namespace,
		},
	}

	cliStartTime, _ := util.GetCLIStartTimeAnnotationValue(resource.ObjectMeta.Annotations)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, repo, func() error {
		if err := controllerutil.SetControllerReference(resource, repo, r.Scheme); err != nil {
			return err
		}

		if repo.ObjectMeta.Annotations == nil {
			repo.ObjectMeta.Annotations = make(map[string]string)
		}
		util.SetCLIStartTimeAnnotationValue(repo.ObjectMeta.Annotations, cliStartTime)

		repo.Spec = v1alpha1.GitRepositorySpec{
			Source: v1alpha1.GitRepositorySource{
				Type:             v1alpha1.SourceTypeRemote,
				RemoteRepository: resource.Spec.RemoteRepository,
				Path:             dirPath,
			},
			GitURL:         resource.Spec.GitServerURL,
			InternalGitURL: resource.Spec.InternalGitServeURL,
			SecretRef:      resource.Spec.GitServerAuthSecretRef,
		}

		return nil
	})

	if err != nil && !errors.IsAlreadyExists(err) {
		return ctrl.Result{}, nil, err
	}

	return ctrl.Result{}, repo, nil
}

func (r *Reconciler) reconcileArgoCDLocalSource(ctx context.Context, resource *v1alpha1.CustomPackage, appName, repoURL string) (ctrl.Result, *v1alpha1.GitRepository, error) {
	logger := log.FromContext(ctx)

	absPath, err := getCNOEAbsPath(resource.Spec.ArgoCD.ApplicationFile, repoURL)
	if err != nil {
		logger.Error(err, "processing argocd app source", "dir", resource.Spec.ArgoCD.ApplicationFile, "repoURL", repoURL)
		return ctrl.Result{}, nil, err
	}

	repo := &v1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      localRepoName(appName, absPath),
			Namespace: resource.Namespace,
		},
	}

	cliStartTime, _ := util.GetCLIStartTimeAnnotationValue(resource.ObjectMeta.Annotations)

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, repo, func() error {
		if err := controllerutil.SetControllerReference(resource, repo, r.Scheme); err != nil {
			return err
		}

		if repo.ObjectMeta.Annotations == nil {
			repo.ObjectMeta.Annotations = make(map[string]string)
		}
		util.SetCLIStartTimeAnnotationValue(repo.ObjectMeta.Annotations, cliStartTime)

		repo.Spec = v1alpha1.GitRepositorySpec{
			Source: v1alpha1.GitRepositorySource{
				Type: v1alpha1.SourceTypeLocal,
				Path: absPath,
			},
			GitURL:         resource.Spec.GitServerURL,
			InternalGitURL: resource.Spec.InternalGitServeURL,
			SecretRef:      resource.Spec.GitServerAuthSecretRef,
		}

		return nil
	})
	// it's possible for an application to specify the same directory multiple times in the spec.
	// if there is a repository already created for this package, no further action is necessary.
	if !errors.IsAlreadyExists(err) {
		return ctrl.Result{}, repo, err
	}

	return ctrl.Result{}, repo, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CustomPackage{}).
		Complete(r)
}

func getArgoCDAppFile(ctx context.Context, resource *v1alpha1.CustomPackage) ([]byte, error) {
	if resource.Spec.RemoteRepository.Url != "" {
		wt, _, err := util.CloneRemoteRepoToMemory(ctx, resource.Spec.RemoteRepository, 1)
		if err != nil {
			return nil, fmt.Errorf("cloning repo, %s: %w", resource.Spec.RemoteRepository.Url, err)
		}
		return util.ReadWorktreeFile(wt, resource.Spec.ArgoCD.ApplicationFile)
	}

	return os.ReadFile(resource.Spec.ArgoCD.ApplicationFile)
}

func localRepoName(appName, dir string) string {
	return fmt.Sprintf("%s-%s", appName, filepath.Base(dir))
}

func remoteRepoName(appName, pathToPkg string, repo v1alpha1.RemoteRepositorySpec) string {
	return fmt.Sprintf("%s-%s", appName, filepath.Base(pathToPkg))
}

func isCNOEScheme(repoURL string) bool {
	return strings.HasPrefix(repoURL, v1alpha1.CNOEURIScheme)
}

func getCNOEAbsPath(fPath, repoURL string) (string, error) {
	parentDir := filepath.Dir(fPath)
	relativePath := strings.TrimPrefix(repoURL, v1alpha1.CNOEURIScheme)
	absPath, err := filepath.Abs(filepath.Join(parentDir, relativePath))
	if err != nil {
		return "", err
	}

	f, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if !f.IsDir() {
		return "", fmt.Errorf("path not a directory: %s", absPath)
	}
	return absPath, err
}
