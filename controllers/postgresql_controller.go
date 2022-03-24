/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbv1 "my.domain/postgres/api/v1"
)

// PostgreSQLReconciler reconciles a PostgreSQL object
type PostgreSQLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=db.my.domain,resources=postgresqls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=db.my.domain,resources=postgresqls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=db.my.domain,resources=postgresqls/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PostgreSQL object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *PostgreSQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	fmt.Println("Reconcile 시작")

	//0. Postgres instance 가져오기
	postgresql := &dbv1.PostgreSQL{}
	if err := r.Get(ctx, req.NamespacedName, postgresql); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// fmt.Println(postgresql)

	//1. configmap 생성
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgres-config",
			Namespace: req.Namespace,
			// Labels: map[string]string{
			// 	"app": "postgres",
			// },
		},
		Data: map[string]string{
			"POSTGRES_DB":       "postgresdb",
			"POSTGRES_USER":     "admin",
			"POSTGRES_PASSWORD": "password",
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, configMap, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		configMap.Labels = labels

		return ctrl.SetControllerReference(postgresql, configMap, r.Scheme)
	})

	if err != nil {
		return ctrl.Result{}, err
	}

	// fmt.Println(configMap)

	//2. pvc 생성
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgres-pv-claim",
			Namespace: req.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: postgresql.Spec.Storage.PVCSpec.StorageClassName,
			Resources:        postgresql.Spec.Storage.PVCSpec.Resources,
		},
	}

	// fmt.Println(postgresql.Spec.Storage.PVCSpec.Resources.Requests.Storage())
	// fmt.Println(pvc)

	// rl := corev1.ResourceList{}
	// rl["storage"] = resource.MustParse(*postgresql.Spec.Storage.Size)

	// parseQuantity, err := resource.ParseQuantity(*postgresql.Spec.Storage.Size)

	// if err != nil {
	// 	return ctrl.Result{}, err
	// }

	// pvc.Spec.Resources = corev1.ResourceRequirements{
	// 	Requests: rl,
	// }
	_, err = ctrl.CreateOrUpdate(ctx, r.Client, pvc, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		pvc.Labels = labels

		return ctrl.SetControllerReference(postgresql, pvc, r.Scheme)
	})

	if err != nil {
		return ctrl.Result{}, err
	}

	//3. Statefulset 생성
	stat := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgres-statefulset",
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app": "postgres",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: postgresql.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "postgres",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "postgres",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgres",
							Image: *postgresql.Spec.Version,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5432,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: configMap.Name,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgredb",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "postgredb",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvc.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	fmt.Println(stat)

	// _, err = ctrl.CreateOrUpdate(ctx, r.Client, stat, func() error {

	// 	// copyLabels := postgresql.GetLabels()
	// 	// if copyLabels == nil {
	// 	// 	copyLabels = map[string]string{}
	// 	// }
	// 	// labels := map[string]string{}
	// 	// for k, v := range copyLabels {
	// 	// 	labels[k] = v
	// 	// }
	// 	// pvc.Labels = labels

	// 	return ctrl.SetControllerReference(postgresql, stat, r.Scheme)
	// })

	if err != nil {
		return ctrl.Result{}, err
	}

	//4. service 생성

	fmt.Println("Reconcile 종료")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1.PostgreSQL{}).
		Complete(r)
}
