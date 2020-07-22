package regularuser

import (
	"fmt"
	"testing"

	"github.com/openshift/managed-cluster-validating-webhooks/pkg/testutils"

	"k8s.io/api/admission/v1beta1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	objectStringResource string = `{
		"metadata": {
			"kind": "%s",
			"uid": "%s",
			"creationTimestamp": "2020-05-10T07:51:00Z"
		},
		"users": null
	}`
	objectStringSubResource string = `{
		"metadata": {
			"kind": "%s",
			"uid": "%s",
			"requestSubResource": "%s",
			"creationTimestamp": "2020-05-10T07:51:00Z"
		},
		"users": null
	}`
	objectStringRequestSubResource string = `{
		"metadata": {
			"kind": "%s",
			"uid": "%s",
			"requestSubResource": "%s",
			"creationTimestamp": "2020-05-10T07:51:00Z"
		},
		"users": null
	}`
)

type regularuserTests struct {
	testID            string
	targetSubResource string
	targetKind        string
	targetResource    string
	targetVersion     string
	targetGroup       string
	username          string
	userGroups        []string
	operation         v1beta1.Operation
	skip              bool // skip this particular test?
	skipReason        string
	shouldBeAllowed   bool
}

func runRegularuserTests(t *testing.T, tests []regularuserTests) {

	for _, test := range tests {
		if test.skip {
			t.Logf("SKIP: Skipping test %s: %s", test.testID, test.skipReason)
			continue
		}
		gvk := metav1.GroupVersionKind{
			Group:   test.targetGroup,
			Version: test.targetVersion,
			Kind:    test.targetKind,
		}
		gvr := metav1.GroupVersionResource{
			Group:    test.targetGroup,
			Version:  test.targetVersion,
			Resource: test.targetResource,
		}
		hook := NewWebhook()
		var rawObjString string
		// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#webhook-request-and-response
		if test.targetSubResource != "" {
			switch hook.MatchPolicy() {
			case admissionregv1.Equivalent:
				rawObjString = fmt.Sprintf(objectStringRequestSubResource, test.targetKind, test.testID, test.targetSubResource)
			case admissionregv1.Exact:
				rawObjString = fmt.Sprintf(objectStringSubResource, test.targetKind, test.testID, test.targetSubResource)
			}
		} else {
			rawObjString = fmt.Sprintf(objectStringResource, test.targetKind, test.testID)
		}
		obj := runtime.RawExtension{
			Raw: []byte(rawObjString),
		}

		httprequest, err := testutils.CreateHTTPRequest(hook.GetURI(),
			test.testID,
			gvk, gvr, test.operation, test.username, test.userGroups, obj)
		if err != nil {
			t.Fatalf("%s Expected no error, got %s", test.testID, err.Error())
		}

		response, err := testutils.SendHTTPRequest(httprequest, hook)
		if err != nil {
			t.Fatalf("%s Expected no error, got %s", test.testID, err.Error())
		}

		if response.Allowed != test.shouldBeAllowed {
			t.Fatalf("%s Mismatch: %s (groups=%s) %s %s the %s %s. Test's expectation is that the user %s. Reason %s", test.testID, test.username, test.userGroups, testutils.CanCanNot(response.Allowed), string(test.operation), test.targetKind, test.targetKind, testutils.CanCanNot(test.shouldBeAllowed), response.Result.Reason)
		}
		if response.UID == "" {
			t.Fatalf("%s No tracking UID associated with the response.", test.testID)
		}
	}
	t.Skip()
}

// TestFirstBlock looks at the first block of permissions in the rules grouping.
// Grouped up here because it's easier this way than to write functions for each
// resource.
// TODO (lisa): In many ways, these follow the same problem as TestInvalidRequest: the Validate method is returning true every time
func TestFirstBlock(t *testing.T) {
	tests := []regularuserTests{
		{
			testID:          "autoscale-priv-user",
			targetResource:  "clusterautoscalers",
			targetKind:      "ClusterAutoscaler",
			targetVersion:   "v1",
			targetGroup:     "autoscaling.openshift.io",
			username:        "kube:system",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Create,
			shouldBeAllowed: true,
		},
		{
			testID:          "autoscale-unpriv-user",
			targetResource:  "clusterautoscalers",
			targetKind:      "ClusterAutoscaler",
			targetVersion:   "v1",
			targetGroup:     "autoscaling.openshift.io",
			username:        "test-user",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Create,
			shouldBeAllowed: false,
		},
	}
	runRegularuserTests(t, tests)
}

// TestInvalidRequest a hook that isn't handled by this hook
func TestInvalidRequest(t *testing.T) {
	tests := []regularuserTests{
		{
			testID:          "node-unpriv-user",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "kube:system",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			skip:            true,
			skipReason:      "Skipping invalid request because at present, Validate will allow it since it isn't written to check ought but the username",
			shouldBeAllowed: false,
		},
		{
			testID:          "node-no-username",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "",
			userGroups:      []string{""},
			operation:       v1beta1.Delete,
			shouldBeAllowed: false,
		},
	}
	runRegularuserTests(t, tests)
}

// These resources follow a similar pattern with a specific Resource is
// specified, and then some subresources
func TestNodesSubjectPermissionsClusterVersions(t *testing.T) {
	tests := []regularuserTests{
		{
			testID:            "sa-node-status-update",
			targetResource:    "nodes",
			targetSubResource: "status",
			targetKind:        "Node",
			targetVersion:     "v1",
			targetGroup:       "",
			username:          "system:node:ip-10-0-0-1.test",
			userGroups:        []string{"system:nodes", "system:authenticated"},
			operation:         v1beta1.Update,
			shouldBeAllowed:   true,
		},
		{
			testID:          "node-unpriv-user",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "system:unauthenticated",
			userGroups:      []string{"system:unauthenticated"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: false,
		},
		{
			testID:          "node-unpriv-user",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-name",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: false,
		},
		{
			testID:          "node-priv-group",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-name",
			userGroups:      []string{"osd-sre-admins", "system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: true,
		},
		{
			testID:          "node-priv-group2",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-name",
			userGroups:      []string{"osd-sre-cluster-admins", "system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: true,
		},
		{
			testID:          "node-admin-user",
			targetResource:  "nodes",
			targetKind:      "Node",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "kube:admin",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: true,
		},
		{
			testID:          "clusterversions-admin-user",
			targetResource:  "clusterversions",
			targetKind:      "ClusterVersion",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "kube:admin",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: true,
		},
		{
			testID:          "clusterversions-priv-user",
			targetResource:  "clusterversions",
			targetKind:      "ClusterVersion",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-user",
			userGroups:      []string{"osd-sre-admins", "system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: true,
		},
		{
			testID:          "clusterversions-unpriv-user",
			targetResource:  "clusterversions",
			targetKind:      "ClusterVersion",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-user",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Delete,
			shouldBeAllowed: false,
		},
		{
			testID:          "subjectpermission-admin-user",
			targetResource:  "subjectpermissions",
			targetKind:      "SubjectPermission",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "kube:admin",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Create,
			shouldBeAllowed: true,
		},
		{
			testID:          "subjectpermission-admin-user",
			targetResource:  "subjectpermissions",
			targetKind:      "SubjectPermission",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-user",
			userGroups:      []string{"osd-sre-admins", "system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Create,
			shouldBeAllowed: true,
		},
		{
			testID:          "clusterversions-unpriv-user",
			targetResource:  "subjectpermissions",
			targetKind:      "SubjectPermission",
			targetVersion:   "v1",
			targetGroup:     "",
			username:        "my-user",
			userGroups:      []string{"system:authenticated", "system:authenticated:oauth"},
			operation:       v1beta1.Create,
			shouldBeAllowed: false,
		},
		{
			testID:            "clusterversions-priv-user-subres",
			targetResource:    "subjectpermissions",
			targetSubResource: "status",
			targetKind:        "SubjectPermission",
			targetVersion:     "v1",
			targetGroup:       "",
			username:          "my-user",
			userGroups:        []string{"osd-sre-admins", "system:authenticated", "system:authenticated:oauth"},
			operation:         v1beta1.Update,
			shouldBeAllowed:   true,
		},
	}
	runRegularuserTests(t, tests)
}

func TestName(t *testing.T) {
	if NewWebhook().Name() == "" {
		t.Fatalf("Empty hook name")
	}
}

func TestRules(t *testing.T) {
	if len(NewWebhook().Rules()) == 0 {
		t.Log("No rules for this webhook?")
	}
}

func TestGetURI(t *testing.T) {
	if NewWebhook().GetURI()[0] != '/' {
		t.Fatalf("Hook URI does not begin with a /")
	}
}
