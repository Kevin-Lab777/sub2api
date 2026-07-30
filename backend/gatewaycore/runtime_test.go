package gatewaycore

import (
	"context"
	"errors"
	"testing"
)

func TestInvocationValidate(t *testing.T) {
	valid := Invocation{
		PoolID:    42,
		RequestID: "req-123",
		SessionID: "session-123",
		Protocol:  ProtocolOpenAI,
		Model:     "gpt-5.4",
		OnUsage: func(context.Context, Usage) error {
			return nil
		},
	}

	tests := []struct {
		name       string
		invocation Invocation
		wantErr    bool
	}{
		{name: "valid", invocation: valid},
		{name: "missing pool", invocation: withPoolID(valid, 0), wantErr: true},
		{name: "missing request ID", invocation: withRequestID(valid, ""), wantErr: true},
		{name: "missing session ID", invocation: withSessionID(valid, ""), wantErr: true},
		{name: "unsupported protocol", invocation: withProtocol(valid, Protocol("other")), wantErr: true},
		{name: "missing model", invocation: withModel(valid, ""), wantErr: true},
		{name: "missing usage callback", invocation: withUsageCallback(valid, nil), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.invocation.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInvocation) {
					t.Fatalf("Validate() error = %v, want ErrInvalidInvocation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func withPoolID(invocation Invocation, poolID int64) Invocation {
	invocation.PoolID = poolID
	return invocation
}

func withRequestID(invocation Invocation, requestID string) Invocation {
	invocation.RequestID = requestID
	return invocation
}

func withSessionID(invocation Invocation, sessionID string) Invocation {
	invocation.SessionID = sessionID
	return invocation
}

func withProtocol(invocation Invocation, protocol Protocol) Invocation {
	invocation.Protocol = protocol
	return invocation
}

func withModel(invocation Invocation, model string) Invocation {
	invocation.Model = model
	return invocation
}

func withUsageCallback(invocation Invocation, callback func(context.Context, Usage) error) Invocation {
	invocation.OnUsage = callback
	return invocation
}
