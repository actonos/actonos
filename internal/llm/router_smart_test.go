package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type dummyProvider struct {
	id       string
	failKind map[TaskKind]bool
	latency  time.Duration
}

func (d *dummyProvider) Complete(ctx context.Context, messages []Message, opts CompletionOptions) (*Response, error) {
	if d.latency > 0 {
		time.Sleep(d.latency)
	}
	if d.failKind != nil && d.failKind[opts.TaskKind] {
		return nil, errors.New("simulated task kind failure")
	}
	return &Response{Content: "dummy response", Model: d.id}, nil
}

func (d *dummyProvider) StreamComplete(ctx context.Context, messages []Message, opts CompletionOptions) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{DeltaContent: "dummy stream", Done: true}
	close(ch)
	return ch, nil
}

func (d *dummyProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (d *dummyProvider) ModelCatalog() []ModelSpec {
	return nil
}

func (d *dummyProvider) ModelName() string {
	return d.id
}

func TestSmartCascadeRouter_TaskKindTrackingAndRetune(t *testing.T) {
	router := NewModelCascadeRouter()

	p1 := &dummyProvider{id: "anthropic/claude-sonnet-4.5", failKind: map[TaskKind]bool{TaskKindClassify: true}}
	p2 := &dummyProvider{id: "google/gemini-2.5-flash"}

	router.RegisterProvider("anthropic/claude-sonnet-4.5", p1)
	router.RegisterProvider("google/gemini-2.5-flash", p2)

	ctx := context.Background()
	cascade := []string{"anthropic/claude-sonnet-4.5", "google/gemini-2.5-flash"}

	// 1. Execute classify task: p1 fails, falls back to p2
	resp, err := router.CompleteWithCascade(ctx, cascade, []Message{{Role: RoleUser, Content: "classify this"}}, CompletionOptions{
		TaskKind: TaskKindClassify,
	})
	if err != nil {
		t.Fatalf("CompleteWithCascade failed: %v", err)
	}
	if resp.Model != "google/gemini-2.5-flash" {
		t.Fatalf("expected fallback to gemini-2.5-flash, got %s", resp.Model)
	}

	// 2. Health report check
	health := router.GetHealthReport()
	if len(health) < 2 {
		t.Fatalf("expected >= 2 health reports, got %d", len(health))
	}

	var p1Health *ProviderHealthReport
	for i := range health {
		if health[i].ProviderID == "anthropic/claude-sonnet-4.5" {
			p1Health = &health[i]
		}
	}
	if p1Health == nil || p1Health.TaskStats[TaskKindClassify].Failures == 0 {
		t.Fatalf("expected p1 to have recorded classify failure in health report: %+v", p1Health)
	}
}

func TestSmartCascadeRouter_HealthProbe(t *testing.T) {
	router := NewModelCascadeRouter()

	p1 := &dummyProvider{id: "provider-a"}
	p2 := &dummyProvider{id: "provider-b"}

	router.RegisterProvider("provider-a", p1)
	router.RegisterProvider("provider-b", p2)

	results := router.RunSelfHealthProbe(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 probe results, got %d", len(results))
	}
	if results["provider-a"] != nil || results["provider-b"] != nil {
		t.Fatalf("expected all healthy probes, got: %v", results)
	}
}
