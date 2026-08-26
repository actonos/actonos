package system

import "testing"

func TestOTAApplySupport(t *testing.T) {
	docker := func() bool { return true }
	none := func() bool { return false }

	if ok, reason := otaApplySupport("linux", "docker", none); ok || reason != ReasonDocker {
		t.Fatalf("RUNTIME_MODE=docker: ok=%v reason=%s", ok, reason)
	}
	if ok, reason := otaApplySupport("linux", "", docker); ok || reason != ReasonDocker {
		t.Fatalf("/.dockerenv: ok=%v reason=%s", ok, reason)
	}
	if ok, reason := otaApplySupport("darwin", "", none); ok || reason != ReasonDarwin {
		t.Fatalf("darwin: ok=%v reason=%s", ok, reason)
	}
	if ok, reason := otaApplySupport("linux", "", none); !ok || reason != "" {
		t.Fatalf("linux bare-metal: ok=%v reason=%s", ok, reason)
	}
	if ok, reason := otaApplySupport("windows", "", none); !ok || reason != "" {
		t.Fatalf("windows native: ok=%v reason=%s", ok, reason)
	}
}

func TestEmbeddingdRequiredNeverUsesNilPointer(t *testing.T) {
	if EmbeddingdRequired(EmbeddingRequiredInput{}) {
		t.Fatal("empty evidence must not require embeddingd")
	}
	if !EmbeddingdRequired(EmbeddingRequiredInput{ServiceReady: true}) {
		t.Fatal("ServiceReady must require embeddingd")
	}
	if !EmbeddingdRequired(EmbeddingRequiredInput{PriorEmbeddingActive: "/data/releases/v1/embeddingd"}) {
		t.Fatal("prior embedding_active must require embeddingd")
	}
	if EmbeddingdRequired(EmbeddingRequiredInput{ServiceReady: true, EnvForce: "0"}) {
		t.Fatal("ACTONOS_OTA_EMBEDDINGD=0 must override")
	}
	if !EmbeddingdRequired(EmbeddingRequiredInput{EnvForce: "1"}) {
		t.Fatal("ACTONOS_OTA_EMBEDDINGD=1 must require")
	}
}
