package agent

import (
	"context"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

func TestCronScheduler_RegisterAndList(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()

	cs := NewCronScheduler(nil, eb)
	cs.Start(context.Background())
	defer cs.Stop()

	job := CronJob{
		ID:              "daily_briefing",
		AgentID:         "agent_default",
		Name:            "Daily Morning Briefing",
		CronExpr:        "0 8 * * *",
		Prompt:          "Generate morning weather & tasks summary",
		TargetChannel:   "telegram",
		TargetRecipient: "123456",
		Enabled:         true,
	}

	if err := cs.RegisterJob(job); err != nil {
		t.Fatalf("failed to register job: %v", err)
	}

	jobs := cs.ListJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	if jobs[0].ID != "daily_briefing" || jobs[0].NextRun.IsZero() {
		t.Errorf("job not properly scheduled: %+v", jobs[0])
	}

	// Invalid cron expression
	invalidJob := CronJob{
		ID:       "invalid_job",
		CronExpr: "not a valid cron",
		Enabled:  true,
	}
	if err := cs.RegisterJob(invalidJob); err == nil {
		t.Fatal("expected error for invalid cron expr, got nil")
	}

	// Remove job
	cs.RemoveJob("daily_briefing")
	if len(cs.ListJobs()) != 0 {
		t.Fatal("expected 0 jobs after removal")
	}
}

func TestCronScheduler_ExecutionTrigger(t *testing.T) {
	eb := bus.NewEventBus()
	defer eb.Close()

	received := false
	sub := eb.Subscribe(bus.EventAgentActionDone)
	defer eb.Unsubscribe(bus.EventAgentActionDone, sub)

	cs := NewCronScheduler(nil, eb)

	job := CronJob{
		ID:              "test_proactive",
		AgentID:         "agent_test",
		Name:            "Test Job",
		CronExpr:        "* * * * *",
		Prompt:          "Ping",
		TargetChannel:   "telegram",
		TargetRecipient: "chat_999",
		Enabled:         true,
	}

	// Directly trigger execution
	go cs.executeJob(&job)

	select {
	case evt := <-sub:
		if evt.AgentID == "agent_test" {
			received = true
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for proactive cron event on eventbus")
	}

	if !received {
		t.Fatal("expected proactive cron event to be received")
	}
}
