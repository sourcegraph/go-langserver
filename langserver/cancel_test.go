package langserver

import (
	"context"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestCancel(t *testing.T) {
	c := &cancel{}
	id1 := jsonrpc2.ID{Num: 1}
	id2 := jsonrpc2.ID{Num: 2}
	id3 := jsonrpc2.ID{Num: 3}
	ctx1, cancel1 := c.WithCancel(context.Background(), id1)
	ctx2, cancel2 := c.WithCancel(context.Background(), id2)
	ctx3, cancel3 := c.WithCancel(context.Background(), id3)

	if ctx1.Err() != nil {
		t.Fatal("ctx1 should not be canceled yet")
	}
	if ctx2.Err() != nil {
		t.Fatal("ctx2 should not be canceled yet")
	}
	if ctx3.Err() != nil {
		t.Fatal("ctx3 should not be canceled yet")
	}

	cancel1()
	if ctx1.Err() == nil {
		t.Fatal("ctx1 should be canceled")
	}
	if ctx2.Err() != nil {
		t.Fatal("ctx2 should not be canceled yet")
	}
	if ctx3.Err() != nil {
		t.Fatal("ctx3 should not be canceled yet")
	}

	c.Cancel(id2)
	if ctx2.Err() == nil {
		t.Fatal("ctx2 should be canceled")
	}
	if ctx3.Err() != nil {
		t.Fatal("ctx3 should not be canceled yet")
	}
	// we always need to call cancel from a WithCancel, even if it is
	// already cancelled. Calling to ensure no panic/etc
	cancel2()

	cancel3()
	if ctx3.Err() == nil {
		t.Fatal("ctx3 should be canceled")
	}
	// If we try to cancel something that has already been cancelled, it
	// should just be a noop.
	c.Cancel(id3)
}
