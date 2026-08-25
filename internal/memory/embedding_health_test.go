package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClaimSkipsWhenHelperUnhealthyAndRevivesDead(t *testing.T) {
	db, service, embedder := newEmbeddingTestService(t)
	ctx := context.Background()
	if err := service.EnqueueMessage(ctx, "m1", "agent_a", "conv1"); err != nil {
		t.Fatal(err)
	}
	_, _ = db.SQLDB().Exec(`UPDATE embedding_jobs SET due_at = ?`, time.Now().UTC().Add(-time.Minute))

	embedder.healthErr = errors.New("embeddingd down")
	job, err := service.claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job != nil {
		t.Fatal("must not claim (or burn attempts) while helper is unhealthy")
	}

	_, _ = db.SQLDB().Exec(`UPDATE embedding_jobs SET status = 'dead', attempts = 8`)
	embedder.healthErr = nil
	service.reviveDeadJobs(ctx)
	job, err = service.claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected revived dead job to be claimable after helper recovery")
	}
}
