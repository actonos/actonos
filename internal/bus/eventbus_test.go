package bus

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	b := NewEventBus()
	defer b.Close()

	chAgent := b.Subscribe(EventAgentCreated)
	chAll := b.Subscribe("*")

	event := NewEvent(EventAgentCreated, "agent_01", map[string]string{"name": "Architect"})
	b.Publish(event)

	select {
	case received := <-chAgent:
		if received.AgentID != "agent_01" {
			t.Fatalf("expected agent_id 'agent_01', got '%s'", received.AgentID)
		}
		if received.Type != EventAgentCreated {
			t.Fatalf("expected type '%s', got '%s'", EventAgentCreated, received.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for specific event")
	}

	select {
	case received := <-chAll:
		if received.AgentID != "agent_01" {
			t.Fatalf("expected agent_id 'agent_01' on wildcard, got '%s'", received.AgentID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for wildcard event")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	b := NewEventBus()
	defer b.Close()

	ch := b.Subscribe(EventTokenRefreshed)
	b.Unsubscribe(EventTokenRefreshed, ch)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestEventBus_ConcurrentPublish(t *testing.T) {
	b := NewEventBus()
	defer b.Close()

	ch := b.Subscribe(EventToolExecutionResult)

	var wg sync.WaitGroup
	count := 50
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			b.Publish(NewEvent(EventToolExecutionResult, "agent_test", idx))
		}(i)
	}

	wg.Wait()

	receivedCount := 0
	for {
		select {
		case <-ch:
			receivedCount++
		case <-time.After(50 * time.Millisecond):
			goto done
		}
	}

done:
	if receivedCount != count {
		t.Fatalf("expected %d received events, got %d", count, receivedCount)
	}
}
