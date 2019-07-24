// Copyright 2019 RedHat
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"context"
	"strconv"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("userMetrics")
)

// GenerateService returns the static service at specified port
func GenerateService(port int32, portName string) (*v1.Service, error) {
	operatorName, err := k8sutil.GetOperatorName()
	if err != nil {
		return nil, err
	}
	namespace, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    map[string]string{"name": operatorName},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port:     port,
					Protocol: v1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: port,
					},
					Name: portName,
				},
			},
			Selector: map[string]string{"name": operatorName},
		},
	}

	return service, nil
}

// GenerateServiceMonitor generates a prometheus-operator ServiceMonitor object
// based on the passed Service object.
func GenerateServiceMonitor(s *v1.Service) *monitoringv1.ServiceMonitor {
	labels := make(map[string]string)
	for k, v := range s.ObjectMeta.Labels {
		labels[k] = v
	}
	return &monitoringv1.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceMonitor",
			APIVersion: "monitoring.coreos.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.ObjectMeta.Name,
			Namespace: s.ObjectMeta.Namespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: s.Spec.Ports[0].Name,
				},
			},
		},
	}
}

// GenerateRoute creates a route to expose the metrics based on the specified path.
func GenerateRoute(s *v1.Service, path string) *routev1.Route {
	log.Info("Staring to generate route modified")
	labels := make(map[string]string)
	for k, v := range s.ObjectMeta.Labels {
		labels[k] = v
	}
	return &routev1.Route{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Route",
			APIVersion: "route.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.ObjectMeta.Name,
			Namespace: s.ObjectMeta.Namespace,
			Labels:    labels,
		},
		Spec: routev1.RouteSpec{
			Path: path,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: s.ObjectMeta.Name,
			},
			Port: &routev1.RoutePort{
				TargetPort: s.Spec.Ports[0].TargetPort,
			},
		},
	}
}

// ConfigureMetrics takes the input values from the user, starts the metrics server,
// as well as crestes service and routes.
func ConfigureMetrics(ctx context.Context, userMetricsConfig metricsConfig) error {
	log.Info("Starting prometheus metrics")

	StartMetrics(userMetricsConfig)

	client, err := createClient()
	if err != nil {
		log.Info("Failed to create new client", "Error", err.Error())
		return err
	}

	p, err := strconv.ParseInt(userMetricsConfig.metricsPort, 10, 32)
	if err != nil {
		return err
	}
	res := int32(p)
	s, svcerr := GenerateService(res, "metrics")
	if svcerr != nil {
		log.Info("Error generating metrics service object.", "Error", svcerr.Error())
		return svcerr
	}
	log.Info("Generated metrics service object")

	// Create or update Service
	_, err = createOrUpdateService(ctx, client, s)
	if err != nil {
		log.Info("Error getting current metrics service", "Error", err.Error())
		return err
	}

	log.Info("Created Service")
	// Generate Route Object
	r := GenerateRoute(s, userMetricsConfig.metricsPath)
	log.Info("Generated metrics route object")

	// Create or Update route
	_, err = createOrUpdateRoute(ctx, client, r)
	if err != nil {
		log.Info("Error creating route", "Error", err.Error())
		return err
	}
	return nil
}

// createOrUpdateService creates or Updates a service object
// which selects the pods from the operator which was deployed.
func createOrUpdateService(ctx context.Context, client client.Client, s *v1.Service) (*v1.Service, error) {
	if err := client.Create(ctx, s); err != nil {
		if !k8serr.IsAlreadyExists(err) {
			return nil, err
		}
		// Service already exists, we want to update it
		// as we do not know if any fields might have changed.
		existingService := &v1.Service{}
		err := client.Get(ctx, types.NamespacedName{
			Name:      s.Name,
			Namespace: s.Namespace,
		}, existingService)

		s.ResourceVersion = existingService.ResourceVersion
		if existingService.Spec.Type == v1.ServiceTypeClusterIP {
			s.Spec.ClusterIP = existingService.Spec.ClusterIP
		}
		err = client.Update(ctx, s)
		if err != nil {
			log.Info("Error creating service object", "Error", err)
			return nil, err
		}
		log.Info("Metrics Service object updated Service.Name %v and Service.Namespace %v", s.Name, s.Namespace)
		return existingService, nil
	}

	log.Info("Metrics Service object created Service.Name %v and Service.Namespace %v", s.Name, s.Namespace)
	return s, nil
}

//createOrUpdateRoute is a function which creates or updates the route for the service object.
func createOrUpdateRoute(ctx context.Context, client client.Client, r *routev1.Route) (*routev1.Route, error) {
	if err := client.Create(ctx, r); err != nil {
		if err != nil {
			if !k8serr.IsAlreadyExists(err) {
				return nil, err
			}

			existingRoute := &routev1.Route{}
			err := client.Get(ctx, types.NamespacedName{
				Name:      r.Name,
				Namespace: r.Namespace,
			}, existingRoute)
			// update the Route
			r.ResourceVersion = existingRoute.ResourceVersion
			if err = client.Update(ctx, r); err != nil {
				log.Info("Error creating metrics route", "Error", err.Error())
				return nil, err
			}
			log.Info("Metrics Route object updated Route.Name %v and Route.Namespace %v", r.Name, r.Namespace)
			return existingRoute, nil
		}

	}
	log.Info("Metrics Route object Created", "Route.Name", r.Name, "Route.Namespace", r.Namespace)
	return r, nil

}

func createClient() (client.Client, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	client, err := client.New(config, client.Options{})
	if err != nil {
		return nil, err
	}

	return client, nil
}
