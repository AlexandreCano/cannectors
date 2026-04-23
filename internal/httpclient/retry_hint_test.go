package httpclient

import (
	"strings"
	"testing"
)

func TestCompileRetryHint_Empty(t *testing.T) {
	prog, err := CompileRetryHint("")
	if err != nil {
		t.Fatalf("empty expression should return (nil, nil), got err=%v", err)
	}
	if prog != nil {
		t.Errorf("empty expression should return nil program")
	}
}

func TestCompileRetryHint_Valid(t *testing.T) {
	prog, err := CompileRetryHint("body.retry == true")
	if err != nil {
		t.Fatalf("valid expression rejected: %v", err)
	}
	if prog == nil {
		t.Fatal("valid expression returned nil program")
	}
}

func TestCompileRetryHint_TooLong(t *testing.T) {
	expr := strings.Repeat("a", MaxRetryHintExpressionLength+1)
	_, err := CompileRetryHint(expr)
	if err == nil {
		t.Fatal("over-long expression should be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("error message missing length hint: %v", err)
	}
}

func TestCompileRetryHint_InvalidSyntax(t *testing.T) {
	_, err := CompileRetryHint("body.retry ==")
	if err == nil {
		t.Fatal("invalid expression should fail to compile")
	}
}

func TestEvalRetryHint_NilProgram(t *testing.T) {
	retry, hinted := EvalRetryHint(nil, []byte(`{"retry": true}`))
	if retry || hinted {
		t.Errorf("nil program must yield (false,false), got (%v,%v)", retry, hinted)
	}
}

func TestEvalRetryHint_EmptyBody(t *testing.T) {
	prog, _ := CompileRetryHint("body.retry == true")
	retry, hinted := EvalRetryHint(prog, nil)
	if retry || hinted {
		t.Errorf("empty body must yield (false,false), got (%v,%v)", retry, hinted)
	}
}

func TestEvalRetryHint_True(t *testing.T) {
	prog, _ := CompileRetryHint("body.retry == true")
	retry, hinted := EvalRetryHint(prog, []byte(`{"retry": true}`))
	if !hinted {
		t.Fatal("expected hinted=true")
	}
	if !retry {
		t.Error("expected retry=true")
	}
}

func TestEvalRetryHint_False(t *testing.T) {
	prog, _ := CompileRetryHint("body.retry == true")
	retry, hinted := EvalRetryHint(prog, []byte(`{"retry": false}`))
	if !hinted {
		t.Fatal("expected hinted=true (valid bool result)")
	}
	if retry {
		t.Error("expected retry=false")
	}
}

func TestEvalRetryHint_InvalidJSON(t *testing.T) {
	prog, _ := CompileRetryHint("body.retry == true")
	retry, hinted := EvalRetryHint(prog, []byte(`not json`))
	if retry || hinted {
		t.Errorf("invalid JSON must yield (false,false), got (%v,%v)", retry, hinted)
	}
}

func TestEvalRetryHint_NonBooleanResult(t *testing.T) {
	prog, err := CompileRetryHint(`body.retry + 1`)
	if err != nil {
		t.Skipf("expression failed to compile: %v", err)
	}
	_, hinted := EvalRetryHint(prog, []byte(`{"retry": 2}`))
	if hinted {
		t.Error("non-boolean result should yield hinted=false")
	}
}
