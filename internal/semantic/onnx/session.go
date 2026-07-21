// Package onnx wraps onnxruntime_go for embedding inference: session setup
// with input introspection (BERT wants token_type_ids, XLM-R does not),
// masked mean pooling and L2 normalization.
package onnx

import (
	"fmt"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var initOnce sync.Once
var initErr error

// Init loads the shared library once per process (§2.2 프로비저닝).
func Init(ortLibPath string) error {
	initOnce.Do(func() {
		ort.SetSharedLibraryPath(ortLibPath)
		initErr = ort.InitializeEnvironment()
	})
	return initErr
}

// Session is one loaded embedding model.
type Session struct {
	mu         sync.Mutex // ORT sessions are not documented goroutine-safe
	sess       *ort.DynamicAdvancedSession
	inputNames []string
	outputName string
	dim        int
}

// NewSession loads the model and introspects its input signature.
func NewSession(modelPath string, dim int) (*Session, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("introspect %s: %w", modelPath, err)
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("model has no outputs")
	}
	inNames := make([]string, len(inputs))
	for i, in := range inputs {
		inNames[i] = in.Name
	}
	outName := outputs[0].Name
	for _, o := range outputs { // prefer the hidden-state output when named
		if o.Name == "last_hidden_state" {
			outName = o.Name
		}
	}
	sess, err := ort.NewDynamicAdvancedSession(modelPath, inNames, []string{outName}, nil)
	if err != nil {
		return nil, err
	}
	return &Session{sess: sess, inputNames: inNames, outputName: outName, dim: dim}, nil
}

// Embed runs one sequence and returns the L2-normalized masked mean of the
// last hidden state (e5 규약 풀링).
func (s *Session) Embed(ids, mask []int64) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := int64(len(ids))
	shape := ort.NewShape(1, n)
	inputs := make([]ort.Value, 0, len(s.inputNames))
	var toDestroy []ort.Value
	defer func() {
		for _, v := range toDestroy {
			_ = v.Destroy()
		}
	}()

	for _, name := range s.inputNames {
		var data []int64
		switch name {
		case "input_ids":
			data = ids
		case "attention_mask":
			data = mask
		case "token_type_ids":
			data = make([]int64, len(ids)) // zeros
		default:
			return nil, fmt.Errorf("unexpected model input %q", name)
		}
		t, err := ort.NewTensor(shape, data)
		if err != nil {
			return nil, err
		}
		toDestroy = append(toDestroy, t)
		inputs = append(inputs, t)
	}

	outputs := []ort.Value{nil} // let ORT allocate
	if err := s.sess.Run(inputs, outputs); err != nil {
		return nil, err
	}
	out := outputs[0]
	defer out.Destroy()

	tensor, ok := out.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type %T", out)
	}
	dims := tensor.GetShape()
	if len(dims) != 3 || dims[0] != 1 {
		return nil, fmt.Errorf("unexpected output shape %v", dims)
	}
	seq, hidden := int(dims[1]), int(dims[2])
	data := tensor.GetData()

	vec := make([]float32, hidden)
	var count float32
	for t := 0; t < seq && t < len(mask); t++ {
		if mask[t] == 0 {
			continue
		}
		row := data[t*hidden : (t+1)*hidden]
		for j, v := range row {
			vec[j] += v
		}
		count++
	}
	if count > 0 {
		for j := range vec {
			vec[j] /= count
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		inv := float32(1 / math.Sqrt(norm))
		for j := range vec {
			vec[j] *= inv
		}
	}
	return vec, nil
}

// Close releases the session.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess != nil {
		_ = s.sess.Destroy()
		s.sess = nil
	}
}
