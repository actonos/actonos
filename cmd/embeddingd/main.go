//go:build cgo && ORT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	modelID       = "intfloat/multilingual-e5-small"
	modelRevision = "614241f622f53c4eeff9890bdc4f31cfecc418b3"
	dimension     = 384
	maxBatchSize  = 32
	maxTextBytes  = 256 << 10
)

type server struct {
	mu        sync.Mutex
	tokenizer *hftokenizer.Tokenizer
	session   *ort.DynamicAdvancedSession
	inputs    []string
}

func main() {
	listenAddr := flag.String("listen-addr", "127.0.0.1:8091", "Loopback HTTP listen address")
	modelDir := flag.String("model-dir", "./data/models/multilingual-e5-small/"+modelRevision, "Model artifact directory")
	ortLibrary := flag.String("onnxruntime-library", "", "Path to onnxruntime shared library")
	flag.Parse()

	if env := os.Getenv("ACTON_EMBEDDING_LISTEN_ADDR"); env != "" {
		*listenAddr = env
	}
	if env := os.Getenv("ACTON_EMBEDDING_MODEL_DIR"); env != "" {
		*modelDir = env
	}
	if env := os.Getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH"); env != "" {
		*ortLibrary = env
	}
	if !isLoopbackAddress(*listenAddr) {
		slog.Error("embeddingd must bind to a loopback address", "address", *listenAddr)
		os.Exit(1)
	}

	service, err := newServer(*modelDir, *ortLibrary)
	if err != nil {
		slog.Error("failed to initialize embedding model", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", service.handleHealth)
	mux.HandleFunc("POST /embed", service.handleEmbed)
	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       time.Minute,
	}

	go func() {
		slog.Info("embeddingd listening", "address", *listenAddr, "model", modelID, "revision", modelRevision)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("embeddingd server failed", "error", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func newServer(modelDir, ortLibrary string) (*server, error) {
	modelPath := filepath.Join(modelDir, "model.onnx")
	tokenizerPath := filepath.Join(modelDir, "tokenizer.json")
	if ortLibrary != "" {
		ort.SetSharedLibraryPath(ortLibrary)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("initializing ONNX Runtime: %w", err)
	}
	tokenizerData, err := os.ReadFile(tokenizerPath)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("reading tokenizer: %w", err)
	}
	tokenizer, err := hftokenizer.NewFromContent(nil, tokenizerData)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("loading tokenizer: %w", err)
	}
	if err := tokenizer.With(api.EncodeOptions{AddSpecialTokens: true, MaxLen: 512}); err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("configuring tokenizer: %w", err)
	}
	inputInfo, outputInfo, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("inspecting ONNX model: %w", err)
	}
	inputNames := make([]string, 0, len(inputInfo))
	for _, input := range inputInfo {
		switch input.Name {
		case "input_ids", "attention_mask", "token_type_ids":
			inputNames = append(inputNames, input.Name)
		default:
			_ = ort.DestroyEnvironment()
			return nil, fmt.Errorf("unsupported ONNX input %q", input.Name)
		}
	}
	outputName := ""
	for _, output := range outputInfo {
		if output.Name == "last_hidden_state" {
			outputName = output.Name
			break
		}
	}
	if outputName == "" && len(outputInfo) > 0 {
		outputName = outputInfo[0].Name
	}
	if outputName == "" {
		_ = ort.DestroyEnvironment()
		return nil, errors.New("ONNX model has no outputs")
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, []string{outputName}, nil)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("loading ONNX model: %w", err)
	}
	return &server{tokenizer: tokenizer, session: session, inputs: inputNames}, nil
}

func (s *server) Close() {
	if s.session != nil {
		_ = s.session.Destroy()
	}
	_ = ort.DestroyEnvironment()
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready", "model_id": modelID, "model_revision": modelRevision, "dimension": dimension,
	})
}

func (s *server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBatchSize*maxTextBytes)
	var request struct {
		Kind  string   `json:"kind"`
		Texts []string `json:"texts"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if request.Kind != "query" && request.Kind != "passage" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind must be query or passage"})
		return
	}
	if len(request.Texts) == 0 || len(request.Texts) > maxBatchSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid batch size"})
		return
	}
	for index, text := range request.Texts {
		if strings.TrimSpace(text) == "" || len(text) > maxTextBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is empty or too large"})
			return
		}
		request.Texts[index] = request.Kind + ": " + strings.TrimSpace(text)
	}
	embeddings, err := s.embed(r.Context(), request.Texts)
	if err != nil {
		slog.Warn("embedding request failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "embedding failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"embeddings": embeddings})
}

func (s *server) embed(ctx context.Context, texts []string) ([][]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	encoded := make([][]int, len(texts))
	maxSequence := 0
	for index, text := range texts {
		encoded[index] = s.tokenizer.Encode(text)
		if len(encoded[index]) > maxSequence {
			maxSequence = len(encoded[index])
		}
	}
	if maxSequence == 0 || maxSequence > 512 {
		return nil, fmt.Errorf("invalid tokenized sequence length %d", maxSequence)
	}
	batch := len(texts)
	inputIDs := make([]int64, batch*maxSequence)
	attentionMask := make([]int64, batch*maxSequence)
	tokenTypeIDs := make([]int64, batch*maxSequence)
	for row, ids := range encoded {
		for column, id := range ids {
			offset := row*maxSequence + column
			inputIDs[offset] = int64(id)
			attentionMask[offset] = 1
		}
	}
	shape := ort.NewShape(int64(batch), int64(maxSequence))
	idsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, err
	}
	defer idsTensor.Destroy()
	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, err
	}
	defer maskTensor.Destroy()
	typeTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, err
	}
	defer typeTensor.Destroy()

	outputs := []ort.Value{nil}
	runOptions, err := ort.NewRunOptions()
	if err != nil {
		return nil, err
	}
	defer runOptions.Destroy()
	done := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-ctx.Done():
			_ = runOptions.Terminate()
		case <-done:
		}
	}()
	inputs := make([]ort.Value, 0, len(s.inputs))
	for _, inputName := range s.inputs {
		switch inputName {
		case "input_ids":
			inputs = append(inputs, idsTensor)
		case "attention_mask":
			inputs = append(inputs, maskTensor)
		case "token_type_ids":
			inputs = append(inputs, typeTensor)
		}
	}
	err = s.session.RunWithOptions(inputs, outputs, runOptions)
	close(done)
	<-watchDone
	if err != nil {
		return nil, err
	}
	defer outputs[0].Destroy()
	output, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected ONNX output type %T", outputs[0])
	}
	outputShape := output.GetShape()
	if len(outputShape) != 3 || outputShape[0] != int64(batch) || outputShape[2] != dimension {
		return nil, fmt.Errorf("unexpected ONNX output shape %v", outputShape)
	}
	data := output.GetData()
	embeddings := make([][]float32, batch)
	for row := range batch {
		vector := make([]float32, dimension)
		count := 0
		for token := 0; token < maxSequence; token++ {
			if attentionMask[row*maxSequence+token] == 0 {
				continue
			}
			base := (row*maxSequence + token) * dimension
			for column := range dimension {
				vector[column] += data[base+column]
			}
			count++
		}
		var norm float64
		for column := range dimension {
			vector[column] /= float32(count)
			norm += float64(vector[column] * vector[column])
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			return nil, fmt.Errorf("model returned zero vector")
		}
		for column := range dimension {
			vector[column] /= float32(norm)
		}
		embeddings[row] = vector
	}
	return embeddings, nil
}

func isLoopbackAddress(address string) bool {
	host := address
	if index := strings.LastIndex(address, ":"); index >= 0 {
		host = strings.Trim(address[:index], "[]")
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
