/*
Copyright 2021.

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

	// add for bgdeploy-xds4
	"fmt"
	appsv1 "k8s.io/api/apps/v1" // add for Depoyment
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr" // add for TargetPort
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	// add for bgdeploy-xds4

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	swallowlabv1alpha1 "bgdeploy-xds4/api/v1alpha1"
)

// BGDeployReconciler reconciles a BGDeploy object
type BGDeployReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=bgdeploy.swallowlab.com,resources=bgdeploys,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bgdeploy.swallowlab.com,resources=bgdeploys/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bgdeploy.swallowlab.com,resources=bgdeploys/finalizers,verbs=update
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BGDeploy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *BGDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// your logic here
	//        reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling BGDeploy")
	//        return ctrl.Result{}, nil
	// }

	// Fetch the BGDeploy instance
	instance := &swallowlabv1alpha1.BGDeploy{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// RECONCILE BLOCK BEGIN

	// Reconcilation Policy:
	//
	// Always
	//      bring-up  bgdeploy-pod-envoy, bgdeploy-svc-xds, and bgdeploy-svc-envoy if they are not running there

	// IF active==blue && transit==OFF {
	//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
	//      terminame bgdeploy-dep-green and bgdeploy-svc-green if they are running there
	//      }

	// ELSE IF active==blue && transit==ON  {
	//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
	//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
	//      update XDS snapshot with direction=blue  if the direction is not blue
	//      }

	// ELSE IF active==green && transit==ON  {
	//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
	//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
	//      update XDS snapshot with direction=green if the direction is not green
	//      }

	// ELSE IF active==green && transit==OFF {
	//      terminate bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
	//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
	//      }

	blueImage := instance.Spec.Blue
	greenImage := instance.Spec.Green
	//              port        := instance.Spec.Port
	//              replicas    := instance.Spec.Replicas
	transitFlag := strings.ToUpper(instance.Spec.Transit) // ON or OFF
	activeApp := strings.ToUpper(instance.Spec.Active)    // BLUE or GREEN

	//                ctx := context.TODO()

	//                pods := &corev1.PodList{}
	//              deps := &appsv1.DeploymentList{}
	//                svcs := &corev1.ServiceList{}

	// Instance Name
	bgdeploy_pod_envoy := instance.Name + "-pod-envoy"
	//        bgdeploy_pod_blue   := instance.Name + "-pod-blue"
	//              bgdeploy_pod_green  := instance.Name + "-pod-green"
	bgdeploy_dep_blue := instance.Name + "-dep-blue"
	bgdeploy_dep_green := instance.Name + "-dep-green"
	bgdeploy_svc_xds := instance.Name + "-svc-xds"
	bgdeploy_svc_envoy := instance.Name + "-svc-envoy"
	bgdeploy_svc_blue := instance.Name + "-svc-blue"
	bgdeploy_svc_green := instance.Name + "-svc-green"

	// Label
	l_bgdeploy_pod_envoy := map[string]string{"app": "bgdeploy", "service": "envoy"}
	//        l_bgdeploy_pod_blue   := map[string]string{ "app" : "bgdeploy" , "color"   : "blue"  }
	//              l_bgdeploy_pod_green  := map[string]string{ "app" : "bgdeploy" , "color"   : "green" }
	l_bgdeploy_dep_blue := map[string]string{"app": "bgdeploy", "color": "blue"}
	l_bgdeploy_dep_green := map[string]string{"app": "bgdeploy", "color": "green"}
	l_bgdeploy_svc_xds := map[string]string{"app": "bgdeploy", "service": "xds"}
	l_bgdeploy_svc_envoy := map[string]string{"app": "bgdeploy", "service": "envoy"}
	l_bgdeploy_svc_blue := map[string]string{"app": "bgdeploy", "color": "blue"}
	l_bgdeploy_svc_green := map[string]string{"app": "bgdeploy", "color": "green"}

	// --------------------------
	podfound := &corev1.Pod{}
	depfound := &appsv1.Deployment{}
	svcfound := &corev1.Service{}
	xdsfound := &corev1.Service{}

	// Always
	//      bring-up  bgdeploy-pod-envoy, bgdeploy-svc-xds, and bgdeploy-svc-envoy if they are not running there

	// Check if the envoy pod already exists, if not create a new one
	envoyPod := r.newEnvoyPodForCR(instance, bgdeploy_pod_envoy, l_bgdeploy_pod_envoy)
	//        podfound := &corev1.Pod{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_pod_envoy, Namespace: instance.Namespace}, podfound)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating envoy Pod", "Pod.Namespace", envoyPod.Namespace, "Pod.Name", envoyPod.Name)
		err = r.Client.Create(context.TODO(), envoyPod)
	}

	// Check if the envoy service already exists, if not create a new one
	envoySvc := r.newEnvoyServiceForCR(instance, bgdeploy_svc_envoy, l_bgdeploy_svc_envoy)
	//        svcfound := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_envoy, Namespace: instance.Namespace}, svcfound)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating envoy Service", "Svc.Namespace", envoySvc.Namespace, "Svc.Name", envoySvc.Name)
		err = r.Client.Create(context.TODO(), envoySvc)
	}

	// Check if the xds service already exists, if not create a new one
	xdsSvc := r.newXDSServiceForCR(instance, bgdeploy_svc_xds, l_bgdeploy_svc_xds)
	//        xdsfound := &corev1.Service{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_xds, Namespace: instance.Namespace}, xdsfound)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating xds Service", "Xds.Namespace", xdsSvc.Namespace, "Xds.Name", xdsSvc.Name)
		err = r.Client.Create(context.TODO(), xdsSvc)
	}

	// IF active==blue && transit==OFF {
	//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
	//      terminame bgdeploy-dep-green and bgdeploy-svc-green if they are running there
	//      }
	if activeApp == "BLUE" && transitFlag == "OFF" {

		// Check if the blue deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_blue, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_blue)
			blueDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_blue, blueImage, l_bgdeploy_dep_blue)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
			err = r.Client.Create(context.TODO(), blueDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the blue service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_blue, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			blueSvc := r.newBGServiceForCR(instance, bgdeploy_svc_blue, l_bgdeploy_svc_blue)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", blueSvc.Namespace, "Xds.Name", blueSvc.Name)
			err = r.Client.Create(context.TODO(), blueSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", blueSvc.Namespace, "Deployment.Name", blueSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green deployment already exists, if yes delete that
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_green, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Green Deployment not found")
		} else {
			reqLogger.Info("Deleting the Deployment")
			if err := r.Client.Delete(context.TODO(), depfound); err != nil {
				reqLogger.Error(err, "failed to delete Deployment resource")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		// Check if the green service already exists, if yes delete that
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_green, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Green Service not found")
		} else {
			reqLogger.Info("Deleting the Service")
			if err := r.Client.Delete(context.TODO(), svcfound); err != nil {
				reqLogger.Error(err, "failed to delete Service resource")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		// set the upstreamURL&Port to the Blue Service
		reqURL := fmt.Sprintf("http://%s.%s:18080/xds?host=%s.%s&port=5000", bgdeploy_svc_xds, instance.Namespace, bgdeploy_svc_blue, instance.Namespace)
		reqLogger.Info("Changing upstreamURL: " + reqURL)
		if _, err := http.Get(reqURL); err != nil {
			reqLogger.Error(err, "failed to set the upstreamURL to Service: ", bgdeploy_svc_blue)
			return ctrl.Result{}, err
		}

	} else if activeApp == "BLUE" && transitFlag == "ON" {

		// ELSE IF active==blue && transit==ON  {
		//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
		//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
		//      update XDS snapshot with direction=blue  if the direction is not blue
		//      }

		// Check if the blue deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_blue, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_blue)
			blueDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_blue, blueImage, l_bgdeploy_dep_blue)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
			err = r.Client.Create(context.TODO(), blueDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the blue service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_blue, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			blueSvc := r.newBGServiceForCR(instance, bgdeploy_svc_blue, l_bgdeploy_svc_blue)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", blueSvc.Namespace, "Xds.Name", blueSvc.Name)
			err = r.Client.Create(context.TODO(), blueSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", blueSvc.Namespace, "Deployment.Name", blueSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_green, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_green)
			greenDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_green, greenImage, l_bgdeploy_dep_green)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
			err = r.Client.Create(context.TODO(), greenDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_green, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			greenSvc := r.newBGServiceForCR(instance, bgdeploy_svc_green, l_bgdeploy_svc_green)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", greenSvc.Namespace, "Xds.Name", greenSvc.Name)
			err = r.Client.Create(context.TODO(), greenSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", greenSvc.Namespace, "Deployment.Name", greenSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// set the upstreamURL&Port to the Blue Service
		reqURL := fmt.Sprintf("http://%s.%s:18080/xds?host=%s.%s&port=5000", bgdeploy_svc_xds, instance.Namespace, bgdeploy_svc_blue, instance.Namespace)
		reqLogger.Info("Changing upstreamURL: " + reqURL)
		if _, err := http.Get(reqURL); err != nil {
			reqLogger.Error(err, "failed to set the upstreamURL to Service: ", bgdeploy_svc_blue)
			return ctrl.Result{}, err
		}

	} else if activeApp == "GREEN" && transitFlag == "ON" {

		// ELSE IF active==green && transit==ON  {
		//      bring-up  bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
		//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
		//      update XDS snapshot with direction=green if the direction is not green
		//      }

		// Check if the blue deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_blue, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_blue)
			blueDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_blue, blueImage, l_bgdeploy_dep_blue)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
			err = r.Client.Create(context.TODO(), blueDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", blueDep.Namespace, "Deployment.Name", blueDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the blue service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_blue, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			blueSvc := r.newBGServiceForCR(instance, bgdeploy_svc_blue, l_bgdeploy_svc_blue)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", blueSvc.Namespace, "Xds.Name", blueSvc.Name)
			err = r.Client.Create(context.TODO(), blueSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", blueSvc.Namespace, "Deployment.Name", blueSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_green, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_green)
			greenDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_green, greenImage, l_bgdeploy_dep_green)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
			err = r.Client.Create(context.TODO(), greenDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_green, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			greenSvc := r.newBGServiceForCR(instance, bgdeploy_svc_green, l_bgdeploy_svc_green)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", greenSvc.Namespace, "Xds.Name", greenSvc.Name)
			err = r.Client.Create(context.TODO(), greenSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", greenSvc.Namespace, "Deployment.Name", greenSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// set the upstreamURL&Port to the Green Service
		reqURL := fmt.Sprintf("http://%s.%s:18080/xds?host=%s.%s&port=5000", bgdeploy_svc_xds, instance.Namespace, bgdeploy_svc_green, instance.Namespace)
		reqLogger.Info("Changing upstreamURL: " + reqURL)
		if _, err := http.Get(reqURL); err != nil {
			reqLogger.Error(err, "failed to set the upstreamURL to Service: ", bgdeploy_svc_green)
			return ctrl.Result{}, err
		}

	} else if activeApp == "GREEN" && transitFlag == "OFF" {

		// ELSE IF active==green && transit==OFF {
		//      terminate bgdeploy-dep-blue  and bgdeploy-svc-blue  if they are not running there
		//      bring-up  bgdeploy-dep-green and bgdeploy-svc-green if they are not running there
		//      }

		// Check if the green deployment already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_green, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			// Define a new deployment
			reqLogger.Info("Defining a new Deployment for: " + bgdeploy_dep_green)
			greenDep := r.newBGDeploymentForCR(instance, bgdeploy_dep_green, greenImage, l_bgdeploy_dep_green)
			reqLogger.Info("Creating a App Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
			err = r.Client.Create(context.TODO(), greenDep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", greenDep.Namespace, "Deployment.Name", greenDep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the green service already exists, if not create a new one
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_green, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			greenSvc := r.newBGServiceForCR(instance, bgdeploy_svc_green, l_bgdeploy_svc_green)
			reqLogger.Info("Creating xds Service", "Xds.Namespace", greenSvc.Namespace, "Xds.Name", greenSvc.Name)
			err = r.Client.Create(context.TODO(), greenSvc)
			if err != nil {
				reqLogger.Error(err, "Failed to create new Service", "Deployment.Namespace", greenSvc.Namespace, "Deployment.Name", greenSvc.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			reqLogger.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}

		// Check if the blue deployment already exists, if yes delete that
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_dep_blue, Namespace: instance.Namespace}, depfound)
		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Blue Deployment not found")
		} else {
			reqLogger.Info("Deleting the Deployment")
			if err := r.Client.Delete(context.TODO(), depfound); err != nil {
				reqLogger.Error(err, "failed to delete Deployment resource")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		// Check if the blue service already exists, if yes delete that
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: bgdeploy_svc_blue, Namespace: instance.Namespace}, svcfound)
		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Blue Service not found")
		} else {
			reqLogger.Info("Deleting the Service")
			if err := r.Client.Delete(context.TODO(), svcfound); err != nil {
				reqLogger.Error(err, "failed to delete Service resource")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		// set the upstreamURL&Port to the Green Service
		reqURL := fmt.Sprintf("http://%s.%s:18080/xds?host=%s.%s&port=5000", bgdeploy_svc_xds, instance.Namespace, bgdeploy_svc_green, instance.Namespace)
		reqLogger.Info("Changing upstreamURL: " + reqURL)
		if _, err := http.Get(reqURL); err != nil {
			reqLogger.Error(err, "failed to set the upstreamURL to Service: ", bgdeploy_svc_green)
			return ctrl.Result{}, err
		}

	}

	// Deployment and Service already exist - don't requeue
	//        reqLogger.Info("Skip reconcile: Deployment and Service already exists", "Deployment.Name", depfound.Name, "Service.Name", svcfound.Name)
	return ctrl.Result{}, nil

}

//     Create Blue or Green Deployment if it isn't there
// newBGDeploymentForCR returns a busybox pod with the same name/namespace as the cr
func (r *BGDeployReconciler) newBGDeploymentForCR(cr *swallowlabv1alpha1.BGDeploy, dep_name string, image_name string, bg_label map[string]string) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep_name,
			Namespace: cr.Namespace,
			Labels:    bg_label,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: bg_label,
			},
			Replicas: &cr.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: bg_label},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "bgdeploy",
							Image: image_name,
							Ports: []corev1.ContainerPort{{
								//                                ContainerPort: &cr.Spec.Port,
								ContainerPort: 5000,
							}},
							Env: []corev1.EnvVar{
								{
									Name:      "K8S_NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
								},
								{
									Name:      "K8S_POD_NAME",
									ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
								},
								{
									Name:      "K8S_POD_IP",
									ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
								},
							},
						},
					},
				},
			},
		},
	}
	controllerutil.SetControllerReference(cr, dep, r.Scheme)
	return dep
}

//     Create Blue or Green Service if it isn't there
func (r *BGDeployReconciler) newBGServiceForCR(cr *swallowlabv1alpha1.BGDeploy, svc_name string, bg_label map[string]string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc_name,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Protocol:   "TCP",
				Port:       5000,
				TargetPort: intstr.FromInt(5000),
			}},
			Type:     corev1.ServiceTypeNodePort,
			Selector: bg_label,
		},
	}
	controllerutil.SetControllerReference(cr, svc, r.Scheme)
	return svc
}

// If Envoy Pod is not there, create a new one
// newPodForCR returns a busybox pod with the same name/namespace as the cr
func (r *BGDeployReconciler) newEnvoyPodForCR(cr *swallowlabv1alpha1.BGDeploy, pod_name string, pod_label map[string]string) *corev1.Pod {

	configmapvolume := &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: pod_name + "-configmap"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod_name,
			Namespace: cr.Namespace,
			Labels:    pod_label,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "envoy",
				Image:   "envoyproxy/envoy:v1.20-latest",
				Command: []string{"/usr/local/bin/envoy"},
				Args:    []string{"--config-path /etc/envoy/envoy.yaml"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "envoy",
					MountPath: "/etc/envoy",
				}},
				Ports: []corev1.ContainerPort{{
					ContainerPort: 10000,
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "envoy",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: configmapvolume,
				},
			}},
		},
	}
	controllerutil.SetControllerReference(cr, pod, r.Scheme)
	return pod
}

// If Envoy Service is not there, create a new one
func (r *BGDeployReconciler) newEnvoyServiceForCR(cr *swallowlabv1alpha1.BGDeploy, svc_name string, svc_label map[string]string) *corev1.Service {

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc_name,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Protocol:   "TCP",
				Port:       10000,
				TargetPort: intstr.FromInt(10000),
			}},
			Type:     corev1.ServiceTypeNodePort,
			Selector: svc_label,
		},
	}
	controllerutil.SetControllerReference(cr, svc, r.Scheme)
	return svc
}

// If XDS Service is not there, create a new one
func (r *BGDeployReconciler) newXDSServiceForCR(cr *swallowlabv1alpha1.BGDeploy, svc_name string, svc_label map[string]string) *corev1.Service {
	/*
	   xdsports1 := corev1.ServicePort{
	   		{
	   		Name: "xds-grpc",
	   		Protocol: "TCP",
	   		Port: 18000,
	   		TargetPort: intstr.FromInt(18000),
	   }}
	*/
	xdsport1 := corev1.ServicePort{}
	xdsport1.Name = "xds-grpc"
	xdsport1.Protocol = "TCP"
	xdsport1.Port = 18000
	xdsport1.TargetPort = intstr.FromInt(18000)

	xdsport2 := corev1.ServicePort{}
	xdsport2.Name = "snapshot-http"
	xdsport2.Protocol = "TCP"
	xdsport2.Port = 18080
	xdsport2.TargetPort = intstr.FromInt(18080)

	/*
	   xdsports2 := corev1.ServicePort{{
	   		Name: "snapshot-http",
	   		Protocol: "TCP",
	   		Port: 18080,
	   		TargetPort: intstr.FromInt(18080)}}

	   //    var xdsports [2]corev1.ServicePort = {xdsport1, xdsport2}
	   xdsports :=[]corev1.ServicePort{ xdsport1 }
	   xdsports =append(xdsports, xdsport2 )

	   xdsports := []corev1.ServicePort{}
	   xdsports[0] = xdsport1
	   xdsports[1] = xdsport2
	*/
	xdsports := []corev1.ServicePort{xdsport1}
	xdsports = append(xdsports, xdsport2)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc_name,
			Namespace: cr.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: xdsports,
			Type:  corev1.ServiceTypeNodePort,
			//            Type: corev1.ServiceTypeClusterIP,
			Selector: svc_label,
		},
	}
	controllerutil.SetControllerReference(cr, svc, r.Scheme)
	return svc
}

// newLabelsForCR returns the labels for selecting the resources
// belonging to the given CR name.
func newLabelsForCR(name string) map[string]string {
	return map[string]string{"app": "bgdeploy", "bgdeploy_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// RECONCILE BLOCK END

// SetupWithManager sets up the controller with the Manager.
func (r *BGDeployReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&swallowlabv1alpha1.BGDeploy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
