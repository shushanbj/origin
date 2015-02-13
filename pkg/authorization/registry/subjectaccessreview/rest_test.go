package subjectaccessreview

import (
	"errors"
	"reflect"
	"testing"
	"time"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/auth/user"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"

	authorizationapi "github.com/openshift/origin/pkg/authorization/api"
	"github.com/openshift/origin/pkg/authorization/authorizer"
)

type subjectAccessTest struct {
	authorizer    *testAuthorizer
	reviewRequest *authorizationapi.SubjectAccessReview
}

type testAuthorizer struct {
	allowed bool
	reason  string
	err     string

	actualAttributes *authorizer.DefaultAuthorizationAttributes
}

func (a *testAuthorizer) Authorize(passedAttributes authorizer.AuthorizationAttributes) (allowed bool, reason string, err error) {
	attributes, ok := passedAttributes.(*authorizer.DefaultAuthorizationAttributes)
	if !ok {
		return false, "ERROR", errors.New("unexpected type for test")
	}

	a.actualAttributes = attributes

	if len(a.err) == 0 {
		return a.allowed, a.reason, nil
	}
	return a.allowed, a.reason, errors.New(a.err)
}
func (a *testAuthorizer) GetAllowedSubjects(passedAttributes authorizer.AuthorizationAttributes) ([]string, []string, error) {
	return nil, nil, nil
}

func TestEmptyReturn(t *testing.T) {
	test := &subjectAccessTest{
		authorizer: &testAuthorizer{
			allowed: false,
			reason:  "because reasons",
		},
		reviewRequest: &authorizationapi.SubjectAccessReview{
			User:     "foo",
			Groups:   []string{},
			Verb:     "get",
			Resource: "pods",
		},
	}

	test.runTest(t)
}

func TestNoErrors(t *testing.T) {
	test := &subjectAccessTest{
		authorizer: &testAuthorizer{
			allowed: true,
			reason:  "because good things",
		},
		reviewRequest: &authorizationapi.SubjectAccessReview{
			Groups:   []string{"master"},
			Verb:     "delete",
			Resource: "deploymentConfigs",
		},
	}

	test.runTest(t)
}

func TestErrors(t *testing.T) {
	test := &subjectAccessTest{
		authorizer: &testAuthorizer{
			err: "some-random-failure",
		},
		reviewRequest: &authorizationapi.SubjectAccessReview{
			User:     "foo",
			Groups:   []string{"first", "second"},
			Verb:     "get",
			Resource: "pods",
		},
	}

	test.runTest(t)
}

func (r *subjectAccessTest) runTest(t *testing.T) {
	const namespace = "unittest"

	storage := REST{r.authorizer}

	expectedResponse := &authorizationapi.SubjectAccessReviewResponse{
		Namespace: namespace,
		Allowed:   r.authorizer.allowed,
		Reason:    r.authorizer.reason,
	}

	expectedAttributes := &authorizer.DefaultAuthorizationAttributes{
		User: &user.DefaultInfo{
			Name:   r.reviewRequest.User,
			Groups: r.reviewRequest.Groups,
		},
		Verb:      r.reviewRequest.Verb,
		Resource:  r.reviewRequest.Resource,
		Namespace: namespace,
	}

	ctx := kapi.WithNamespace(kapi.NewContext(), namespace)
	channel, err := storage.Create(ctx, r.reviewRequest)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	select {
	case result := <-channel:
		switch obj := result.Object.(type) {
		case *kapi.Status:
			if len(r.authorizer.err) == 0 {
				t.Errorf("Unexpected operation error: %v", obj)
			}

		case *authorizationapi.SubjectAccessReviewResponse:
			if !reflect.DeepEqual(expectedResponse, obj) {
				t.Errorf("diff %v", util.ObjectGoPrintDiff(expectedResponse, obj))
			}

		default:
			t.Errorf("Unexpected result type: %v", result)
		}
	case <-time.After(time.Millisecond * 100):
		t.Error("Unexpected timeout from async channel")
	}

	if !reflect.DeepEqual(expectedAttributes, r.authorizer.actualAttributes) {
		t.Errorf("diff %v", util.ObjectGoPrintDiff(expectedAttributes, r.authorizer.actualAttributes))
	}
}
