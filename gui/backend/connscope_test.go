package backend

import (
	"context"
	"testing"
)

func TestConnScopeEndsThePreviousConnection(t *testing.T) {
	var c connScope
	first := c.begin(context.Background())
	second := c.begin(context.Background())
	if first.Err() == nil {
		t.Fatal("a new connection left the previous one's work running")
	}
	if second.Err() != nil {
		t.Fatal("the new connection's work is already over")
	}
	c.end()
	c.end() // twice is fine
	if second.Err() == nil {
		t.Fatal("end did not stop the work")
	}
}
