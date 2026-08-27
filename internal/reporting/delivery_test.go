package reporting

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type reportResolverFunc func(context.Context, string, string) ([]net.IP, error)

func (f reportResolverFunc) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return f(ctx, network, host)
}

type reportDoerFunc func(*http.Request) (*http.Response, error)

func (f reportDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

func TestDeliveryWorkerPostsAndCompletesSubmission(t *testing.T) {
	t.Parallel()
	service, memory := newReportingService()
	service.resolver = reportResolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	var calls atomic.Int32
	service.doer = reportDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Idempotency-Key") == "" || request.Header.Get("X-DokoSoko-Submission-Kind") != KindFeedback {
			t.Fatalf("unexpected request: method=%s headers=%v", request.Method, request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || !strings.Contains(string(body), `"kind":"feedback"`) {
			t.Fatalf("delivery body=%s err=%v", body, err)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})

	created, err := service.SubmitFeedback(context.Background(), FeedbackInput{Message: "Useful integration guide", IdempotencyKey: "delivery-worker-idempotency-1"}, submitContext())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.RunDelivery(ctx, time.Millisecond) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, loadErr := memory.ReportSubmission(context.Background(), "prod_acme", created.ID)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if stored.State == "delivered" {
			if stored.Attempts != 1 || stored.DeliveredAt == nil || calls.Load() != 1 {
				t.Fatalf("stored=%#v calls=%d", stored, calls.Load())
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatal("delivery worker did not complete the submission")
}

func TestDeliveryRejectsUnsafeDestinationsAndPermanentFailures(t *testing.T) {
	t.Parallel()
	service, _ := newReportingService()
	service.resolver = reportResolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	service.doer = reportDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe destination must not be contacted")
		return nil, nil
	})
	err := service.deliver(context.Background(), reportSubmissionFixture("https://support.example.test/feedback"))
	var failure *deliveryFailure
	if !strings.Contains(err.Error(), "unsafe") || !asDeliveryFailure(err, &failure) || failure.retryable {
		t.Fatalf("unsafe delivery error=%#v", err)
	}

	service.resolver = reportResolverFunc(func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	service.doer = reportDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("rejected")), Header: make(http.Header)}, nil
	})
	err = service.deliver(context.Background(), reportSubmissionFixture("https://support.example.test/feedback"))
	if !asDeliveryFailure(err, &failure) || failure.retryable || failure.category != "delivery returned HTTP 400" {
		t.Fatalf("permanent delivery error=%#v", err)
	}
}

func asDeliveryFailure(err error, destination **deliveryFailure) bool {
	failure, ok := err.(*deliveryFailure)
	if ok {
		*destination = failure
	}
	return ok
}

func reportSubmissionFixture(destination string) model.ReportSubmission {
	return model.ReportSubmission{ID: "submission-1", Kind: KindFeedback, DeliveryURL: destination, Payload: []byte(`{"kind":"feedback"}`)}
}
