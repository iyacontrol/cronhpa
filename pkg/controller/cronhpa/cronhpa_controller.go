package cronhpa

import (
	"context"
	"fmt"
	"time"

	"github.com/iyacontrol/config-hpa/pkg/apis"
	confighpav1beta1 "github.com/iyacontrol/config-hpa/pkg/apis/confighpa/v1beta1"
	cronutil "github.com/robfig/cron"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	chpav1beta1 "github.com/iyacontrol/cronhpa/pkg/apis/cronhpa/v1beta1"
)

const (
	defaultSyncPeriod = time.Second * 60
)

var sche = runtime.NewScheme()

// Add creates a new cronhpa Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	clientConfig := mgr.GetConfig()

	clientSet, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		log.Fatal(err)
	}

	options := ctrl.Options{Scheme: sche}
	configHpaClientSet, err := client.New(clientConfig, client.Options{Scheme: options.Scheme})
	if err != nil {
		log.Fatal(err)
	}

	evtNamespacer := clientSet.CoreV1()
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(log.Infof)
	broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: evtNamespacer.Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "cron-horizontal-pod-autoscaler"})

	return &ReconcileCHPA{
		Client:             mgr.GetClient(),
		scheme:             mgr.GetScheme(),
		clientSet:          clientSet,
		configHpaClientSet: configHpaClientSet,
		eventRecorder:      recorder,
		syncPeriod:         defaultSyncPeriod,
	}
}

var _ reconcile.Reconciler = &ReconcileCHPA{}

// ReconcileCHPA reconciles a CHPA object
type ReconcileCHPA struct {
	client.Client
	//replicaCalculator *podautoscaler.ReplicaCalculator
	scheme        *runtime.Scheme
	syncPeriod    time.Duration
	eventRecorder record.EventRecorder

	clientSet          kubernetes.Interface
	configHpaClientSet client.Client
}

// When the CHPA is changed (status is changed, edited by the user, etc),
// a new "UpdateEvent" is generated and passed to the "updatePredicate" function.
// If the function returns "true", the event is added to the "Reconcile" queue,
// If the function returns "false", the event is skipped.
func updatePredicate(ev event.UpdateEvent) bool {
	oldObject := ev.ObjectOld.(*chpav1beta1.CronHPA)
	newObject := ev.ObjectNew.(*chpav1beta1.CronHPA)
	// Add the chpa object to the queue only if the spec has changed.
	// Status change should not lead to a requeue.
	if !apiequality.Semantic.DeepEqual(newObject.Spec, oldObject.Spec) {
		return true
	}
	return false
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cron-hpa-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to CHPA
	predicate := predicate.Funcs{UpdateFunc: updatePredicate}
	err = c.Watch(&source.Kind{Type: &chpav1beta1.CronHPA{}}, &handler.EnqueueRequestForObject{}, predicate)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CHPA object and makes changes based on the state read
// and what is in the CHPA.Spec
// The implementation repeats kubernetes hpa implementation from v1.10.8

// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// TODO: decide, what to use: patch or update in rbac
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;update
// +kubebuilder:rbac:groups=confighpa.shareit.com,resources=confighpas,verbs=get;update
// +kubebuilder:rbac:groups=cronhpa.shareit.com,resources=cronhpas,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileCHPA) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("")
	log.Infof("Reconcile request: %v\n", request)

	// resRepeat will be returned if we want to re-run reconcile process
	// NB: we can't return non-nil err, as the "reconcile" msg will be added to the rate-limited queue
	// so that it'll slow down if we have several problems in a row
	resRepeat := reconcile.Result{RequeueAfter: r.syncPeriod}
	// resStop will be returned in case if we found some problem that can't be fixed, and we want to stop repeating reconcile process
	resStop := reconcile.Result{}

	chpa := &chpav1beta1.CronHPA{}
	err := r.Get(context.TODO(), request.NamespacedName, chpa)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Do not repeat the Reconcile again
			return resStop, nil
		}
		// Error reading the object (intermittent problems?) - requeue the request,
		log.Errorf("Can't get cronhpa object '%v': %v", request.NamespacedName, err)
		return resRepeat, nil
	}

	if err := checkCHPAValidity(chpa); err != nil {
		log.Errorf("Got an invalid CHPA spec '%v': %v", request.NamespacedName, err)
		// The chpa spec is incorrect (most likely, in "metrics" section) stop processing it
		// When the spec is updated, the chpa will be re-added to the reconcile queue
		r.eventRecorder.Event(chpa, v1.EventTypeWarning, "FailedSpecCheck", err.Error())
		return resStop, nil
	}
	if err := r.reconcileCHPA(chpa); err != nil {
		// Should never happen, actually.
		log.Errorf(err.Error())
		r.eventRecorder.Event(chpa, v1.EventTypeWarning, "FailedProcessCHPA", err.Error())
		return resStop, nil
	}

	return resRepeat, nil
}

// Function returns an error only when we need to stop working with the CHPA spec
func (r *ReconcileCHPA) reconcileCHPA(chpa *chpav1beta1.CronHPA) (err error) {
	defer func() {
		if err1 := recover(); err1 != nil {
			err = fmt.Errorf("RunTime error in reconcileCHPA: %s", err1)
		}
	}()

	now := time.Now()
	latestSchedledTime := getLatestScheduledTime(chpa)

	for _, cron := range chpa.Spec.Crons {
		sched, err := cronutil.ParseStandard(cron.Schedule)
		if err != nil {
			log.Errorf("Unparseable schedule: %s : %s", cron.Schedule, err)
		}
		t := sched.Next(latestSchedledTime)
		log.Infof("Next schedule for %s of cronhpa %s: %v", cron.Schedule, getCronHPAFullName(chpa), t)
		if !t.After(now) {
			log.Infof("Scale %s to min replicas %d for schedule %s", getCronHPAFullName(chpa),
				cron.TargetReplicas, cron.Schedule)
			// Set new replicas
			if err := r.scale(chpa, cron.TargetReplicas); err != nil {
				log.Errorf("Failed to scale %s to min replicas %d: %v", getCronHPAFullName(chpa), cron.TargetReplicas, err)
				return err
			}
			// update status
			hpaStatusOriginal := chpa.Status.DeepCopy()
			chpa.Status.LastScheduleTime = &metav1.Time{Time: time.Now()}

			if apiequality.Semantic.DeepEqual(hpaStatusOriginal, &chpa.Status) {
				return nil
			}

			//hpaRaw, err := unsafeConvertToVersionVia(chpa, confighpav1beta1.SchemeGroupVersion)
			//if err != nil {
			//	r.eventRecorder.Eventf(chpa, v1.EventTypeWarning, "FailedConvertHPA", err.Error())
			//	return fmt.Errorf("failed to convert the given HPA to %s: %v", confighpav1beta1.SchemeGroupVersion.String(), err)
			//}
			//
			//hpav1 := hpaRaw.(*chpav1beta1.CronHPA)

			if err := r.Client.Status().Update(context.TODO(), chpa); err != nil {
				log.Errorf("Failed to update cronhpa %s's LastScheduleTime(%+v): %v",
					getCronHPAFullName(chpa), chpa.Status.LastScheduleTime.Time, err)
				return err
			}
			log.Infof("Successfully updated status for %s", getCronHPAFullName(chpa))
			return nil

		}
	}

	return nil
}

func (r *ReconcileCHPA) scale(cronhpa *chpav1beta1.CronHPA, replicas int32) error {
	reference := fmt.Sprintf("%s/%s/%s", cronhpa.Spec.ScaleTargetRef.Kind, cronhpa.Namespace, cronhpa.Spec.ScaleTargetRef.Name)

	switch cronhpa.Spec.ScaleTargetRef.Kind {
	case "HorizontalPodAutoscaler":
		hpa, err := r.clientSet.AutoscalingV2beta1().HorizontalPodAutoscalers(cronhpa.Namespace).Get(cronhpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get native hpa %s: %v", reference, err)
		}

		oldReplicas := hpa.Spec.MinReplicas
		hpa.Spec.MinReplicas = &replicas

		_, err = r.clientSet.AutoscalingV2beta1().HorizontalPodAutoscalers(cronhpa.Namespace).Update(hpa)
		if err != nil {
			r.eventRecorder.Eventf(cronhpa, v1.EventTypeWarning, "FailedRescale", err.Error())
			return fmt.Errorf("failed to rescale %s: %v", reference, err)
		}
		r.eventRecorder.Eventf(cronhpa, v1.EventTypeNormal, "SuccessfulRescale", "New size: %d", replicas)
		log.Infof("Successful scale of %s, old size: %d, new size: %d",
			getCronHPAFullName(cronhpa), oldReplicas, replicas)

	case "ConfigHpa":
		hpa := &confighpav1beta1.ConfigHpa{}
		err := r.configHpaClientSet.Get(context.TODO(), types.NamespacedName{
			Namespace: cronhpa.Namespace,
			Name:      cronhpa.Spec.ScaleTargetRef.Name,
		}, hpa)
		if err != nil {
			r.eventRecorder.Eventf(cronhpa, v1.EventTypeWarning, "FailedRescale", err.Error())
			return fmt.Errorf("failed to rescale %s: %v", reference, err)
		}

		oldReplicas := hpa.Spec.MinReplicas
		hpa.Spec.MinReplicas = &replicas
		err = r.configHpaClientSet.Update(context.TODO(), hpa)
		if err != nil {
			r.eventRecorder.Eventf(cronhpa, v1.EventTypeWarning, "FailedRescale", err.Error())
			return fmt.Errorf("failed to rescale %s: %v", reference, err)
		}
		r.eventRecorder.Eventf(cronhpa, v1.EventTypeNormal, "SuccessfulRescale", "New size: %d", replicas)
		log.Infof("Successful scale of %s, old size: %d, new size: %d",
			getCronHPAFullName(cronhpa), oldReplicas, replicas)
	}

	return nil
}

func checkCHPAValidity(chpa *chpav1beta1.CronHPA) error {
	if ok := chpa.Spec.ScaleTargetRef.Kind == "HorizontalPodAutoscaler" || chpa.Spec.ScaleTargetRef.Kind == "ConfigHpa"; !ok {
		msg := fmt.Sprintf("configurable chpa doesn't support %s kind, use HorizontalPodAutoscaler or  ConfigHpa", chpa.Spec.ScaleTargetRef.Kind)
		log.Infof(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func getLatestScheduledTime(chpa *chpav1beta1.CronHPA) time.Time {
	if chpa.Status.LastScheduleTime != nil {
		return chpa.Status.LastScheduleTime.Time
	} else {
		return chpa.CreationTimestamp.Time
	}
}

func getCronHPAFullName(chpa *chpav1beta1.CronHPA) string {
	return chpa.Namespace + "/" + chpa.Name
}

func init() {
	apis.AddToScheme(sche)
}
